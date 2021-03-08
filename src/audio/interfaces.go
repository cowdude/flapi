package audio

import "io"

type AudioReader interface {
	io.Reader
}

type AudioWriter interface {
	io.Writer
}

type AudioReadWriter interface {
	io.ReadWriter
}
