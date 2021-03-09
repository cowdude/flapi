package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/cowdude/flapi/src/audio"
)

var (
	thresholdStr = flag.String("threshold", "-20dB",
		"Gain activation threshold in decibels, or raw unit range [0.0;1.0]")
	timeout        = flag.Duration("timeout", time.Millisecond*300, "Activity timeout")
	gainSmooth     = flag.Float64("gain_smooth", 0.9, "EMA weight for estimating average gain")
	bufferDuration = flag.Duration("buffer_duration", time.Second*10,
		"Maximum audio input to keep in memory before it loops back over itself")
	contextPrefix = flag.Duration("context_prefix", time.Millisecond*20,
		"Include the N preceding moment before activation")
	trace = flag.Bool("trace", false, "Enable trace logging")
)

func main() {
	flag.Parse()
	if *trace {
		log.SetLevel(log.TraceLevel)
	}

	var threshold audio.Gain
	err := threshold.UnmarshalYAML(func(out interface{}) error {
		if v, ok := out.(*string); ok {
			*v = *thresholdStr
			return nil
		}
		return os.ErrInvalid
	})
	if err != nil {
		log.Fatalf("Failed to parse threshold: %v", threshold)
	}
	log.Printf("threshold: %v", threshold)

	segments := make(chan audio.Activity, 1)
	info := make(chan audio.WAVEInfo)
	ctx := context.Background()
	go func() {
		defer close(segments)
		defer close(info)
		err := audio.ScanActivity(ctx, os.Stdin, info, segments, audio.ActivityOpts{
			Threshold:       threshold,
			ActivityTimeout: *timeout,
			BufferDuration:  *bufferDuration,
			GainSmooth:      *gainSmooth,
			ContextPrefix:   *contextPrefix,
		})
		if err != nil {
			panic(err)
		}
	}()

	var counter int
	format := <-info
	log.Printf("WAV input format: %+v", format)
	for res := range segments {
		fmt.Printf("start: %v    end: %v    gain: %v\n", res.Start, res.Duration, res.Mean)
		f, err := os.Create(fmt.Sprintf("/tmp/seg_%d.wav", counter))
		counter++
		if err = audio.WriteWAVHeader(f, format); err != nil {
			panic(err)
		}
		if _, err = io.Copy(f, bytes.NewReader(res.Frames)); err != nil {
			panic(err)
		}
		if err = f.Close(); err != nil {
			panic(err)
		}
	}
}
