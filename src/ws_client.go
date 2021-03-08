package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"sync/atomic"

	log "github.com/Sirupsen/logrus"
	"github.com/cowdude/flapi/src/audio"
	"github.com/gorilla/websocket"
)

type Client struct {
	GUID           string
	Conn           *websocket.Conn
	Request        *http.Request
	ReceivingAudio bool

	Audio struct {
		In    *audio.ChanReader
		InfoC chan audio.WAVEInfo

		Activity chan audio.Activity
	}
}

type ClientEvent string

const (
	EStatusChanged ClientEvent = "status_changed"
	EPrediction                = "prediction"
)

type ResponsePayload struct {
	Request string      `json:"request"`
	Success bool        `json:"success"`
	Result  interface{} `json:"result,omitempty"`
	Message string      `json:"message,omitempty"`
}

type EventPayload struct {
	Event   ClientEvent `json:"event"`
	Result  interface{} `json:"result,omitempty"`
	Message string      `json:"message,omitempty"`
}

var clientIDCounter uint64

func (c *Client) SendEvent(e EventPayload) {
	err := c.Conn.WriteJSON(e)
	if err != nil {
		log.Warnf("failed to send event: %v", err)
	}
}

func NewClient(conn *websocket.Conn, r *http.Request) (c *Client) {
	counter := 1 + atomic.AddUint64(&clientIDCounter, 1)
	log.WithField("GUID", counter).Println("Created new client")
	c = &Client{
		GUID:    fmt.Sprintf("%08X", counter),
		Conn:    conn,
		Request: r,
	}

	c.Audio.In = audio.NewChanReader()
	transcoder := audio.Transcode(r.Context(), c.Audio.In, audio.WAV, 16000)
	c.Audio.Activity = make(chan audio.Activity, 1)
	c.Audio.InfoC = make(chan audio.WAVEInfo, 1)

	go func() {
		defer c.Audio.In.Close()
		err := audio.ScanActivity(r.Context(), transcoder, c.Audio.InfoC, c.Audio.Activity, Config.Activity)
		log.WithField("guid", c.GUID).WithError(err).Println("Exited scan goroutine")
	}()
	go func() {
		defer close(c.Audio.Activity)
		defer close(c.Audio.InfoC)
		err := c.run()
		log.WithField("guid", c.GUID).WithError(err).Println("Exited run goroutine")
	}()

	select {
	case <-asrReady:
		c.SendEvent(EventPayload{
			Event:   EStatusChanged,
			Result:  true,
			Message: "ASR ready",
		})
	default:
		c.SendEvent(EventPayload{
			Event:   EStatusChanged,
			Result:  false,
			Message: "ASR still warming up",
		})
	}
	return
}

func (c *Client) handleBinary(data []byte) (err error) {
	if !c.ReceivingAudio {
		return errors.New("got binary data but was not receiving audio")
	}
	log.WithField("bytes", len(data)).Debug("Recv binary message")
	if _, err := c.Audio.In.Write(data); err != nil { //shadowing intentional, dont care.
		log.Warnf("Failed to buffer audio: %v", err) //shortWrite
	}
	return
}

func (c *Client) predict(counter *uint, format audio.WAVEInfo, data []byte) (pred Prediction, err error) {
	f, err := os.Create(path.Join(os.TempDir(), fmt.Sprintf("%v_%04x.wav", c.GUID, *counter)))
	*counter++
	if err != nil {
		log.Panic(err)
	}
	defer os.Remove(f.Name())

	format.FileSize = audio.WAVHeaderSize + uint32(len(data))
	if err = audio.WriteWAVHeader(f, format); err != nil {
		f.Close()
		return
	}
	if _, err = io.Copy(f, bytes.NewReader(data)); err != nil {
		f.Close()
		return
	}
	if err = f.Close(); err != nil {
		return
	}

	log.Debugf("wrote tmp WAV file: %v", f.Name())
	return asr.Predict(f.Name())
}

func (c *Client) run() (err error) {
	defer log.WithField("guid", c.GUID).Print("client runner exited")
	ctx := c.Request.Context()

	var counter uint
	var format audio.WAVEInfo
	select {
	case <-ctx.Done():
		return ctx.Err()
	case format = <-c.Audio.InfoC:
	}

	log.Debugf("ASR input audio format: %+v", format)
	for {
		var prediction Prediction
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event := <-c.Audio.Activity:
			log.Debugf("audio activity: start=%v duration=%v gain=~%v", event.Start, event.Duration, event.Mean)
			if prediction, err = c.predict(&counter, format, event.Frames); err != nil {
				return
			}
			log.Debugf("got prediction: %v", prediction)
			c.SendEvent(EventPayload{
				Event:  EPrediction,
				Result: prediction,
			})
		}
	}
}
