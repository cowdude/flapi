package audio

import "io"

type ChanReader struct {
	v  chan []byte
	at []byte
}

func NewChanReader() *ChanReader {
	return &ChanReader{v: make(chan []byte)}
}

func (buf *ChanReader) Read(dst []byte) (n int, err error) {
	if len(buf.at) == 0 {
		buf.at = <-buf.v
		if buf.at == nil {
			return 0, io.EOF
		}
	}
	n = copy(dst, buf.at)
	buf.at = buf.at[n:]
	return
}

func (buf *ChanReader) Write(src []byte) (n int, err error) {
	if len(src) != 0 {
		buf.v <- src
		n = len(src)
	}
	return
}

func (buf *ChanReader) Close() error {
	close(buf.v)
	return nil
}
