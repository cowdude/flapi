package audio

type ChanReader struct {
	v  chan []byte
	at []byte
}

func NewChanReader() *ChanReader {
	return &ChanReader{v: make(chan []byte)}
}

func (buf *ChanReader) Read(dst []byte) (n int, err error) {
	for n == 0 {
		if len(buf.at) == 0 {
			buf.at = <-buf.v
		}
		n = copy(dst, buf.at)
		buf.at = buf.at[n:]
	}
	return
}

func (buf *ChanReader) Write(src []byte) (n int, err error) {
	buf.v <- src
	n = len(src)
	return
}

func (buf *ChanReader) Close() error {
	close(buf.v)
	return nil
}
