package datura

import (
	"errors"
	"io"
	"sync"
	"unsafe"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/bytedance/sonic/ast"
	"github.com/theapemachine/errnie"
)

type artifactStreamState struct {
	readWire      []byte
	readOffset    int
	readDone      bool
	writeBuffer   []byte
	cache         map[string]any
	indexed       bool
	payloadBytes  []byte
	payloadRoot   ast.Node
	payloadParsed bool
}

var artifactStreamStates sync.Map

func artifactStreamKey(artifact *Artifact) uintptr {
	return uintptr(unsafe.Pointer(artifact))
}

func artifactStreamStateFor(artifact *Artifact) *artifactStreamState {
	key := artifactStreamKey(artifact)

	if existing, ok := artifactStreamStates.Load(key); ok {
		return existing.(*artifactStreamState)
	}

	state := &artifactStreamState{
		cache: make(map[string]any, 8),
	}
	actual, _ := artifactStreamStates.LoadOrStore(key, state)

	return actual.(*artifactStreamState)
}

func resetArtifactStreamState(artifact *Artifact) {
	key := artifactStreamKey(artifact)

	existing, loaded := artifactStreamStates.Load(key)

	if !loaded {
		return
	}

	state := existing.(*artifactStreamState)
	state.readWire = nil
	state.readOffset = 0
	state.readDone = false
	state.writeBuffer = nil
	state.indexed = false
	state.payloadBytes = nil
	state.payloadRoot = ast.Node{}
	state.payloadParsed = false

	for cacheKey := range state.cache {
		delete(state.cache, cacheKey)
	}

	artifactStreamStates.Delete(key)
}

/*
Read implements the io.Reader interface for the Artifact.
It marshals the entire artifact into the provided byte slice.
*/
func (artifact *Artifact) Read(p []byte) (n int, err error) {
	state := artifactStreamStateFor(artifact)

	if state.readDone {
		return 0, io.EOF
	}

	if state.readWire == nil {
		state.readWire, err = artifact.Message().Marshal()

		if err != nil {
			return 0, errnie.Error(err, "p", string(p))
		}

		state.readOffset = 0
	}

	if state.readOffset >= len(state.readWire) {
		state.readWire = nil
		state.readOffset = 0
		state.readDone = true

		return 0, io.EOF
	}

	n = copy(p, state.readWire[state.readOffset:])
	state.readOffset += n

	if state.readOffset >= len(state.readWire) {
		state.readWire = nil
		state.readOffset = 0
		state.readDone = true

		return n, io.EOF
	}

	return n, nil
}

/*
Write implements the io.Writer interface for the Artifact.
It unmarshals the provided bytes into the current artifact.
*/
func (artifact *Artifact) Write(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, errors.New("empty input")
	}

	state := artifactStreamStateFor(artifact)
	state.writeBuffer = append(state.writeBuffer, p...)

	var (
		msg     *capnp.Message
		inbound Artifact
		segment *capnp.Segment
	)

	if msg, err = capnp.Unmarshal(state.writeBuffer); err != nil {
		return len(p), nil
	}

	if inbound, err = ReadRootArtifact(msg); err != nil {
		return 0, errnie.Error(err)
	}

	if segment, err = artifact.Message().Reset(capnp.SingleSegment(nil)); err != nil {
		return 0, errnie.Error(err)
	}

	writable, err := NewRootArtifact(segment)

	if err != nil {
		return 0, errnie.Error(err)
	}

	if err = capnp.Struct(writable).CopyFrom(capnp.Struct(inbound)); err != nil {
		return 0, errnie.Error(err)
	}

	*artifact = writable
	state.writeBuffer = nil
	resetArtifactStreamState(artifact)

	return len(p), nil
}

/*
Close implements the io.Closer interface for the Artifact.
*/
func (artifact *Artifact) Close() error {
	resetArtifactStreamState(artifact)
	artifact = nil

	return nil
}
