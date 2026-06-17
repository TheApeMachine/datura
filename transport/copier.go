package transport

import "io"

func Copy(destination io.Writer, origin io.Reader) (int64, error) {
	return io.Copy(destination, origin)
}
