package transport

import (
	"errors"
	"io"
	"strings"
	"sync"
)

type packer interface {
	Pack() []byte
}

const (
	copierBufferSize          = 4 << 20
	copierMaxBufferSize       = 64 << 20
	copierNoProgressReadLimit = 100
)

var copierPool = sync.Pool{
	New: func() any {
		return make([]byte, copierBufferSize)
	},
}

func Copy(destination io.Writer, origin io.Reader) (int64, error) {
	if source, ok := origin.(packer); ok {
		return copyPacked(destination, source)
	}

	buffer := copierPool.Get().([]byte)

	if len(buffer) != copierBufferSize {
		buffer = make([]byte, copierBufferSize)
	}

	defer copierPool.Put(buffer)

	return copyBuffered(destination, origin, buffer)
}

func copyPacked(destination io.Writer, origin packer) (int64, error) {
	wire := origin.Pack()

	if len(wire) == 0 {
		return 0, io.EOF
	}

	n, err := destination.Write(wire)

	if err != nil {
		return int64(n), err
	}

	if n != len(wire) {
		return int64(n), io.ErrShortWrite
	}

	return int64(n), nil
}

func copyBuffered(destination io.Writer, origin io.Reader, buffer []byte) (int64, error) {
	var written int64
	emptyReads := 0

	for {
		readCount, readErr := origin.Read(buffer)

		if isShortBuffer(readErr) {
			if len(buffer) >= copierMaxBufferSize {
				return written, readErr
			}

			buffer = growBuffer(buffer)
			continue
		}

		if readCount > 0 {
			writeCount, writeErr := destination.Write(buffer[:readCount])
			written += int64(writeCount)

			if writeErr != nil {
				return written, writeErr
			}

			if writeCount != readCount {
				return written, io.ErrShortWrite
			}
		}

		if readErr != nil {
			if readErr == io.EOF {
				if written == 0 {
					return written, io.EOF
				}

				return written, nil
			}

			return written, readErr
		}

		if readCount == 0 {
			emptyReads++

			if emptyReads >= copierNoProgressReadLimit {
				return written, io.ErrNoProgress
			}

			continue
		}

		emptyReads = 0
	}
}

func isShortBuffer(err error) bool {
	if err == nil {
		return false
	}

	return errors.Is(err, io.ErrShortBuffer) ||
		strings.Contains(err.Error(), io.ErrShortBuffer.Error())
}

func growBuffer(buffer []byte) []byte {
	next := len(buffer) * 2

	if next > copierMaxBufferSize {
		next = copierMaxBufferSize
	}

	// ponytail: large staged frames retry in memory; replace with offset-based
	// Artifact.Read streaming if packed frames regularly exceed the max buffer.
	return make([]byte, next)
}
