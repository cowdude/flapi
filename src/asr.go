package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
)

type ASRRunner struct {
	cmd *exec.Cmd

	TX        chan string
	RX        chan Prediction
	waitInput chan struct{}
	wg        sync.WaitGroup
}

type Prediction struct {
	InputFile string `json:"input_file"`
	Text      string `json:"text"`
}

func (runner *ASRRunner) Close() (err error) {
	close(runner.TX)
	for range runner.RX {
	}
	runner.wg.Wait()

	if runner.cmd.ProcessState != nil && !runner.cmd.ProcessState.Exited() {
		log.Error("process leaked during Close")
	}
	return
}

func (runner *ASRRunner) transmit(w io.Writer, line string) (err error) {
	epoch := time.Now()
	for timeout := time.Second * 15; ; timeout *= 2 {
		select {
		case <-runner.waitInput:
			log.Debugf("sending input: %v", line)
			w.Write([]byte(line))
			_, err = w.Write([]byte{'\n'})
			return
		case now := <-time.After(timeout):
			elapsed := now.Sub(epoch)
			log.Warnf("process is falling behind for %v", elapsed.Truncate(time.Second))
		}
	}
}

func (runner *ASRRunner) Run() (err error) {
	var input io.WriteCloser
	var output io.ReadCloser
	if input, err = runner.cmd.StdinPipe(); err != nil {
		return
	}
	if output, err = runner.cmd.StderrPipe(); err != nil {
		return
	}

	log.Debug("starting process")
	if err = runner.cmd.Start(); err != nil {
		return
	}

	runner.wg.Add(3)
	defer runner.wg.Done()
	go func() {
		defer runner.wg.Done()
		defer input.Close()
		for line := range runner.TX {
			if err := runner.transmit(input, line); err != nil {
				log.Errorf("failed to send input to ASR: %v", err)
				return
			}
		}
	}()

	go func() {
		const (
			predictedOutputStr = `[Inference tutorial for CTC]: predicted output for `
			waitingInputStr    = `[Inference tutorial for CTC]: Waiting the input`
		)
		defer runner.wg.Done()
		defer close(runner.RX)
		defer close(runner.waitInput)
		var (
			scanner        = bufio.NewScanner(output)
			readingPred    bool
			predictionFile string
			prediction     strings.Builder
		)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, waitingInputStr) {
				//process is now waiting for input file path
				if readingPred {
					runner.RX <- Prediction{
						InputFile: predictionFile,
						Text:      prediction.String(),
					}
					prediction.Reset()
					predictionFile = ""
					readingPred = false
				}
				log.Debug("[RX]ASR waiting for input")
				runner.waitInput <- struct{}{}
			} else if pos := strings.LastIndex(line, predictedOutputStr); pos != -1 {
				//process is now telling prediction for a given file path
				if readingPred {
					log.Panicf("stdio parse state violation at predicted output: '%v'", line)
				}
				predictionFile = strings.TrimSpace(line[pos+len(predictedOutputStr):])
				readingPred = true
			} else if readingPred {
				//process is telling prediction
				if prediction.Len() != 0 {
					prediction.WriteRune('\n')
				}
				prediction.WriteString(line)
			} else {
				//unparsed process logs
				fmt.Println(line) // don't feed a logger entry into another logger...
			}
		}
		if err := scanner.Err(); err != nil {
			log.Error(err)
		}
	}()
	return runner.cmd.Wait()
}

func (runner *ASRRunner) Predict(inputFile string) (res Prediction, err error) {
	epoch := time.Now()
	defer func() {
		elapsed := time.Since(epoch)
		log.Debugf("end-to-end ASR prediction took %v", elapsed)
	}()

	runner.TX <- inputFile
	res = <-runner.RX
	if res.InputFile == "" {
		err = errors.New("asr process exited")
		return
	}
	if res.InputFile != inputFile {
		log.Panicf("Received prediction for '%v' instead of '%v'", res.InputFile, inputFile)
	}
	return
}

func NewRunner() *ASRRunner {
	args := []string{
		`--am_path=` + Config.Flashlight.AccousticModel,
		`--tokens_path=` + Config.Flashlight.Tokens,
		`--lexicon_path=` + Config.Flashlight.Lexicon,
		`--lm_path=` + Config.Flashlight.LanguageModel,
		`--logtostderr=true`,
		`--sample_rate=16000`,
		fmt.Sprintf(`--beam_size=%v`, Config.Flashlight.BeamSize),
		fmt.Sprintf(`--beam_size_token=%v`, Config.Flashlight.BeamSizeToken),
		fmt.Sprintf(`--beam_threshold=%v`, Config.Flashlight.BeamThreshold),
		fmt.Sprintf(`--lm_weight=%v`, Config.Flashlight.LanguageModelWeight),
		fmt.Sprintf(`--word_score=%v`, Config.Flashlight.WordScore),
	}

	log.Debugf("args: %v", strings.Join(args, " "))
	return &ASRRunner{
		cmd:       exec.Command(Config.Flashlight.Executable, args...),
		TX:        make(chan string),
		RX:        make(chan Prediction),
		waitInput: make(chan struct{}),
	}
}
