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
	readWire              []byte
	readOffset            int
	readDone              bool
	writeBuffer           []byte
	retainStageAttributes bool
	indexed               bool
	payloadBytes          []byte
	payloadRoot           ast.Node
	payloadParsed         bool
	attributesBytes       []byte
	attributesRoot        ast.Node
	attributesParsed      bool
	attributesDirty       bool
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

	state := &artifactStreamState{}
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
	state.retainStageAttributes = false
	state.indexed = false
	state.payloadBytes = nil
	state.payloadRoot = ast.Node{}
	state.payloadParsed = false
	state.attributesBytes = nil
	state.attributesRoot = ast.Node{}
	state.attributesParsed = false
	state.attributesDirty = false

	artifactStreamStates.Delete(key)
}

func resetArtifactReadState(state *artifactStreamState) {
	state.readWire = nil
	state.readOffset = 0
	state.readDone = false
	state.writeBuffer = nil
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
		flushAttributesRoot(artifact)
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

	var preserved ast.Node

	if state.retainStageAttributes {
		root, ok := ensureAttributesRoot(artifact)

		if ok {
			preserved = root
		}
	}

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
	state.attributesParsed = false
	state.attributesDirty = false
	resetArtifactReadState(state)

	if !state.retainStageAttributes {
		return len(p), nil
	}

	if errnie.Error(mergeStageAttributes(artifact, preserved)) != nil {
		return 0, errnie.Error(err)
	}

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
