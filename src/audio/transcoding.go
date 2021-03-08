package audio

import (
	"context"
	"strconv"
)

func Transcode(ctx context.Context, src AudioReader, dstFormat EncoderFormat, sampleRate int) FFReader {
	return ffmpegReader(ctx, src,
		"-hide_banner",
		"-nostats",
		"-vn", "-sn", "-dn",
		"-i", "-",
		"-f", string(dstFormat), "-ac", "1", "-ar", strconv.Itoa(sampleRate), "-",
	)
}
