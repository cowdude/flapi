//go:generate go run github.com/UnnoTed/fileb0x b0x.yml

package main

import (
	"flag"
	"fmt"
	"net/http"
	"runtime"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/cowdude/flapi/src/static"
)

var asr *ASRRunner
var verbose = flag.Bool("v", false, "enable debug logging")
var asrReady = make(chan struct{})

func warmup() {
	defer close(asrReady)
	defer DispatchEvent(EventPayload{
		Event:   EStatusChanged,
		Result:  true,
		Message: "ASR is ready",
	})

	if Config.Warmup == nil {
		return
	}
	for i := 0; i < Config.Warmup.Repeat; i++ {
		log.Printf("Warming up (%d/%d) ...", i+1, Config.Warmup.Repeat)
		if pred, err := asr.Predict(Config.Warmup.Audio); err != nil {
			log.Error("warmup prediction failed")
			log.Panic(err)
		} else if strings.ToLower(pred.Text) != strings.ToLower(Config.Warmup.GroundTruth) {
			log.Fatalf("warmup prediction differs from ground truth: %+v", pred)
		}
	}
	log.Println("Warmup complete")
}

var logLabels = make(map[string]string)

func callerPrettifier(f *runtime.Frame) (function string, file string) {
	var ok bool
	function = ""
	if file, ok = logLabels[f.File]; !ok {
		start := strings.LastIndexByte(f.File, '/')
		end := strings.LastIndexByte(f.File, '.')
		file = fmt.Sprintf("[%s]", strings.ToUpper(f.File[start+1:end]))
		logLabels[f.File] = file
	}
	return
}

func init() {
	flag.Parse()
	LoadConfig()

	log.SetFormatter(&log.TextFormatter{
		PadLevelText:     true,
		CallerPrettyfier: callerPrettifier,
	})
	log.SetReportCaller(true)
	if *verbose {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}
	log.Printf("Log level set to %v", log.GetLevel())
	log.Printf("Config: %+v", Config)
}

func main() {
	asr = NewRunner()
	defer asr.Close()

	go func() {
		log.Panic(asr.Run())
	}()
	go warmup()

	http.HandleFunc("/v1/ws", handleWS)
	http.Handle("/", http.FileServer(static.HTTP))
	log.Printf("http listening on %v", Config.HTTP.Listen)
	log.Fatal(http.ListenAndServe(Config.HTTP.Listen, nil))
}
