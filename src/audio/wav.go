package audio

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/glycerine/rbuf"

	log "github.com/sirupsen/logrus"

	"github.com/go-audio/wav"
)

type Gain float64

func Decibels(dB float64) Gain { return Gain(math.Pow(10, dB/10)) }

func (gain Gain) Decibels() float64 { return 10 * math.Log10(float64(gain)) }
func (gain Gain) String() string    { return fmt.Sprintf("%.3g [%.0fdB]", gain, gain.Decibels()) }

type WAVDecoder struct {
	d *wav.Decoder
}

type str4 [4]byte

type PCMBuffer []int16

type WAVEInfo struct {
	FileSize        uint32
	fmt             uint16
	nc              uint16
	sr              uint32
	nAvgBytesPerSec uint32
	nBlockAlign     uint16
	bps             uint16
	dataSize        uint32
}

type waveReader struct {
	scratch [8]byte
	WAVEInfo
	src io.Reader
	err error
}

func (gain *Gain) UnmarshalYAML(unmarshal func(interface{}) error) (err error) {
	var number float64
	var text string

	if err = unmarshal(&number); err == nil {
		*gain = Gain(number)
		return
	}
	if err = unmarshal(&text); err == nil {
		if strings.HasSuffix(text, "dB") {
			number, err = strconv.ParseFloat(text[:len(text)-2], 64)
			if err == nil {
				*gain = Gain(math.Pow(10, number/10))
				return
			}
		}
	}
	return fmt.Errorf("unable to parse %v as gain", text)
}

func (id str4) String() string       { return string(id[:]) }
func (buf PCMBuffer) String() string { return fmt.Sprintf("[PCM len=%v cap=%v]", len(buf), cap(buf)) }

func (wav *waveReader) bytesUnsafe(n int) []byte {
	var read int
	var err error
	read, err = wav.src.Read(wav.scratch[:n])
	if err != nil {
		wav.err = err
		return nil
	}
	if read != n {
		wav.err = io.ErrShortBuffer
		return nil
	}
	return wav.scratch[:n]
}
func (wav *waveReader) bytes(n int) (res []byte) {
	res = make([]byte, n)
	src := wav.bytesUnsafe(n)
	copy(res, src)
	return
}
func (wav *waveReader) str4() (res str4) {
	var read int
	var err error
	read, err = wav.src.Read(res[:])
	if err != nil {
		wav.err = err
		return str4{}
	}
	if read != 4 {
		wav.err = io.ErrShortBuffer
		return str4{}
	}
	return
}
func (wav *waveReader) u16() (res uint16) {
	wav.bytesUnsafe(int(unsafe.Sizeof(res)))
	return *(*uint16)(unsafe.Pointer(&wav.scratch[0]))
}
func (wav *waveReader) u32() (res uint32) {
	wav.bytesUnsafe(int(unsafe.Sizeof(res)))
	return *(*uint32)(unsafe.Pointer(&wav.scratch[0]))
}

var (
	RIFF = str4{'R', 'I', 'F', 'F'}
	WAVE = str4{'W', 'A', 'V', 'E'}
	FMTX = str4{'f', 'm', 't', ' '}
	DATA = str4{'d', 'a', 't', 'a'}
	LIST = str4{'L', 'I', 'S', 'T'}
)

func (wav *waveReader) header() (ok bool) {
	if wav.str4() != RIFF {
		return
	}
	wav.FileSize = wav.u32()
	if wav.str4() != WAVE {
		return
	}
	if wav.str4() != FMTX {
		return
	}

	if wav.u32() != 16 { //cksize (const)
		return
	}
	wav.fmt = wav.u16() //1 for PCM
	wav.nc = wav.u16()
	wav.sr = wav.u32()
	wav.nAvgBytesPerSec = wav.u32()
	wav.nBlockAlign = wav.u16()
	wav.bps = wav.u16()

	for {
		bID := wav.str4()
		bsize := int(wav.u32())
		switch bID {
		case DATA:
			wav.dataSize = uint32(bsize)
			return true
		case LIST:
			for bsize != 0 {
				m := bsize
				if lim := cap(wav.scratch); m > lim {
					m = lim
				}
				n, err := wav.src.Read(wav.scratch[:m])
				if err != nil {
					wav.err = err
					return
				}
				bsize -= n
			}
		default:
			wav.err = fmt.Errorf("unknown block ID '%v'", bID)
			return
		}
	}
}

func (wav *waveReader) sample16() (samples PCMBuffer) {
	const sampleSize = int(unsafe.Sizeof(samples[0]))
	var read int
	var err error
	read, err = wav.src.Read(wav.scratch[:])
	if err != nil {
		wav.err = err
		return nil
	}
	if read%sampleSize != 0 {
		wav.err = io.ErrShortBuffer
		return nil
	}
	sh := reflect.SliceHeader{
		Data: uintptr(unsafe.Pointer(&wav.scratch[0])),
		Len:  read / sampleSize,
		Cap:  read / sampleSize,
	}
	return *(*PCMBuffer)(unsafe.Pointer(&sh))
}

type Activity struct {
	Start    time.Duration
	Duration time.Duration
	Mean     Gain
	Frames   []byte
}

type ActivityOpts struct {
	Threshold       Gain          `yaml:"threshold"`
	GainSmooth      float64       `yaml:"gain_smooth"`
	ActivityTimeout time.Duration `yaml:"timeout"`
	BufferDuration  time.Duration `yaml:"buffer_duration"`
	ContextPrefix   time.Duration `yaml:"context_prefix"`
}

func ScanActivity(ctx context.Context, src AudioReader, nfo chan<- WAVEInfo, c chan<- Activity, opts ActivityOpts) (err error) {
	wav := waveReader{src: src}
	if ok := wav.header(); !ok {
		if err = wav.err; err == nil {
			err = errors.New("unknown error")
		}
	}
	if err != nil {
		return
	}
	nfo <- wav.WAVEInfo
	if wav.nc != 1 {
		log.Panicf("monochannel WAV only, but found %v channels", wav.nc)
	}
	if wav.bps != 16 {
		log.Panicf("16 bits per sample WAV only, but found %d bits", wav.bps)
	}
	if wav.fmt != 1 {
		log.Panicf("PCM-encoded WAV only, got format 0x%X", wav.fmt)
	}
	var (
		atSample            int64
		gainEMA             float64
		meanActiveGain      Gain
		meanActiveGainCount int

		beginActiveFrame    int64 = -1
		beginHigh, beginLow uint32
		endHigh, endLow     uint32
		nHigh, nLow         uint32

		closeNotBefore = int64(math.MaxInt64)
		w0             = math.Pow(2, float64(wav.bps-1))

		activationWindow   = int64(16 * time.Millisecond * time.Duration(wav.sr) / time.Second)
		deactivationWindow = int64(opts.ActivityTimeout * time.Duration(wav.sr) / time.Second)
		contextFrames      = int64(opts.ContextPrefix * time.Duration(wav.sr) / time.Second)

		buffers = make([]*rbuf.FixedSizeRingBuf, 4)
		back    int
	)

	for i := range buffers {
		size := opts.BufferDuration * time.Duration(wav.sr) * time.Duration(wav.bps) / 8 / time.Second
		buffers[i] = rbuf.NewFixedSizeRingBuf(int(size))
	}

	log.Println("parsed WAV header:")
	log.Printf("type: %v", wav.fmt)
	log.Printf("channels: %v", wav.nc)
	log.Printf("sampling rate: %v", wav.sr)
	log.Printf("bits per sample: %v", wav.bps)
	log.Printf("nAvgBytesPerSec: %v", wav.nAvgBytesPerSec)
	log.Printf("nBlockAlign: %v", wav.nBlockAlign)
	log.Printf("context frames: %v", contextFrames)

	nospam := time.NewTicker(time.Second * 5)
	defer nospam.Stop()
	for {
		samples := wav.sample16()
		err = wav.err
		if err == io.EOF {
			if len(samples) != 0 {
				panic("samples !=0 at EOF")
			}
			err = nil
			return
		} else if err != nil {
			return
		}
		if len(samples) == 0 {
			log.Panic()
		}

		select {
		case <-nospam.C:
			log.WithField("atSample", atSample).
				WithField("gain", meanActiveGain/Gain(meanActiveGainCount)).
				WithField("samples", len(samples)).Info("Scanning audio activity")
		default:
		}

		for _, s := range samples {
			instGain := float64(s) / w0
			gainEMA = gainEMA*opts.GainSmooth + instGain*(1-opts.GainSmooth)
			dg := instGain - gainEMA

			var active bool
			if dg >= float64(opts.Threshold) {
				nHigh++
				active = true
			} else if dg <= float64(-opts.Threshold) {
				nLow++
				active = true
			}

			if beginActiveFrame == -1 {
				// outside of active window
				if active {
					log.Tracef("begin active frame at %v: low=%v high=%v", atSample, nLow, nHigh)
					beginLow, beginHigh = nLow, nHigh
					beginActiveFrame = atSample
				}
			} else if atSample-beginActiveFrame == activationWindow {
				// scan first samples of potentially active window
				if nHigh-beginHigh > 1 && nLow-beginLow > 1 {
					// potentially active window is indeed active
					log.Debugf("active window validated at %v", atSample)
					closeNotBefore = atSample + deactivationWindow
					endLow, endHigh = nLow, nHigh
					meanActiveGain = 0
					meanActiveGainCount = 0
				} else {
					log.Tracef("rejected at %v: dlow=%v dhigh=%v", atSample, nLow-beginLow, nHigh-beginHigh)
					beginActiveFrame = -1
					buffers[back].Reset()
				}
			} else if atSample >= closeNotBefore-deactivationWindow && atSample < closeNotBefore {
				// inside active window, look for silence closure
				if nHigh-endHigh > 1 || nLow-endLow > 1 {
					// not silent, postpone closure
					log.Tracef("rejected closure early at %v: dlow=%v dhigh=%v",
						atSample, nLow-beginLow, nHigh-beginHigh)
					closeNotBefore = atSample + deactivationWindow
					endLow, endHigh = nLow, nHigh
				}
			} else if atSample == closeNotBefore {
				// evaluate silence closure
				if nHigh-endHigh <= 1 && nLow-endLow <= 1 {
					//found closure
					log.Debugf("active window closed at %v: dlow=%v dhigh=%v",
						atSample, nLow-endLow, nHigh-endHigh)
					data := buffers[back].Bytes()
					lastn := int((atSample - beginActiveFrame + contextFrames) * 2) //PCM 16bits
					if lastn < 0 {
						lastn = 0
					}
					if lastn > len(data) {
						lastn = len(data)
					}
					frames := data[len(data)-lastn:]
					meanActiveGain /= Gain(meanActiveGainCount)
					c <- Activity{
						Start:    time.Duration(beginActiveFrame) * time.Second / time.Duration(wav.sr),
						Duration: time.Duration(atSample-beginActiveFrame) * time.Second / time.Duration(wav.sr),
						Mean:     meanActiveGain,
						Frames:   frames,
					}
					back = (back + 1) % len(buffers)
					buffers[back].Reset()
					beginActiveFrame = -1
					meanActiveGain = 0
					meanActiveGainCount = 0
				} else {
					log.Tracef("rejected closure at %v: dlow=%v dhigh=%v", atSample, nLow-endLow, nHigh-endHigh)
					closeNotBefore = atSample + deactivationWindow
					endLow, endHigh = nLow, nHigh
				}
			}

			const nbytes = 2 //PCM 16bits
			if wcap := buffers[back].N - buffers[back].Readable; wcap < nbytes {
				//drop oldest record to make some room
				buffers[back].Advance(nbytes - wcap)
			}
			buffers[back].Write(*(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{
				Data: uintptr(unsafe.Pointer(&s)),
				Cap:  nbytes, Len: nbytes,
			})))

			meanActiveGain += Gain(math.Abs(dg))
			meanActiveGainCount++
			atSample++
		}
	}
}

type waveWriter struct {
	WAVEInfo
}

const WAVHeaderSize = 44

func WriteWAVHeader(w AudioWriter, info WAVEInfo) (err error) {
	var header bytes.Buffer
	const cksize uint32 = 16
	header.Write(RIFF[:])
	binary.Write(&header, binary.LittleEndian, info.FileSize)
	header.Write(WAVE[:])
	header.Write(FMTX[:])
	binary.Write(&header, binary.LittleEndian, cksize)
	binary.Write(&header, binary.LittleEndian, info.fmt)
	binary.Write(&header, binary.LittleEndian, info.nc)
	binary.Write(&header, binary.LittleEndian, info.sr)
	binary.Write(&header, binary.LittleEndian, info.nAvgBytesPerSec)
	binary.Write(&header, binary.LittleEndian, info.nBlockAlign)
	binary.Write(&header, binary.LittleEndian, info.bps)
	header.Write(DATA[:])
	binary.Write(&header, binary.LittleEndian, info.dataSize)
	_, err = header.WriteTo(w)
	return
}
