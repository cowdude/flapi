package audio

import (
	"context"
	"os"
	"os/exec"

	log "github.com/sirupsen/logrus"
)

type EncoderFormat string

const (
	WAV   EncoderFormat = "wav"
	LAVFI               = "lavfi"
	FLAC                = "flac"
)

type FFReader struct {
	AudioReader
	lastErr error
	waitErr chan error
}

func ffmpegReader(ctx context.Context, stdin AudioReader, args ...string) (res FFReader) {
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stdin = stdin

	for _, arg := range args {
		if arg == "-" {
			res.AudioReader, res.lastErr = cmd.StdoutPipe()
			if res.lastErr != nil {
				return
			}
			break
		}
	}

	if log.GetLevel() >= log.DebugLevel {
		cmd.Stderr = os.Stderr
	}

	log.Debugf("starting ffmpeg: %v", args)
	res.lastErr = cmd.Start()
	if res.lastErr != nil {
		return
	}

	//watchdog
	res.waitErr = make(chan error)
	go func() {
		res.waitErr <- cmd.Wait()
		close(res.waitErr)
	}()
	return
}

func (reader *FFReader) Wait() error {
	if reader.lastErr == nil {
		reader.lastErr = <-reader.waitErr
	}
	return reader.lastErr
}
