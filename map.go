package datura

import (
	"sync"

	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/ast"
	"github.com/theapemachine/errnie"
)

// Freeze the configuration once at startup to reuse JIT-compiled encoders.
var fastSonic = sonic.Config{
	EncodeNullForInfOrNan: true, // Converts NaN, +Inf, -Inf to JSON `null` instead of returning an error
}.Froze()

var mapPool = sync.Pool{
	New: func() any {
		return make(Map[any], 8)
	},
}

/*
NewMap acquires a pre-allocated Map[any] from the sync.Pool and optionally populates
it with key-value pairs (e.g. NewMap("key1", val1, "key2", val2)).
Call Free() or MarshalAndFree() to recycle it back to the pool.
*/
func NewMap(kv ...any) Map[any] {
	m := mapPool.Get().(Map[any])

	for i := 0; i < len(kv)-1; i += 2 {
		if key, ok := kv[i].(string); ok {
			m[key] = kv[i+1]
		}
	}

	return m
}

type Map[T any] map[string]T

/*
Free clears all entries in the Map and returns it to the sync.Pool.
*/
func (m Map[T]) Free() {
	if m == nil {
		return
	}

	clear(m)
	mapPool.Put(any(m).(Map[any]))
}

func (m Map[T]) Marshal() []byte {
	payload, err := fastSonic.Marshal(m)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation, "datura.Map: failed to marshal payload", err,
		))

		return nil
	}

	return payload
}

/*
MarshalAndFree marshals the Map to JSON via fastSonic and immediately returns
the Map to the sync.Pool.
*/
func (m Map[T]) MarshalAndFree() []byte {
	payload := m.Marshal()
	m.Free()

	return payload
}

func (artifact *Artifact) PokePayload(value any, path ...any) *Artifact {
	root := errnie.Does(func() (ast.Node, error) {
		return sonic.Get(artifact.DecryptPayload())
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "payload peek failed", err))
	}).Value()

	if !root.Exists() {
		root = ast.NewObject(nil)
	}

	root.SetAnyByPath(finite(value), path...)

	payload := errnie.Does(func() ([]byte, error) {
		return root.MarshalJSON()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "payload marshal failed", err))
	}).Value()

	if len(payload) > 0 {
		artifact.WithPayload(payload)
	}

	return artifact
}

/*
Merge writes a key/value into the artifact payload in place, preserving sibling
keys already present. The payload is the data channel; use this for top-level
payload data (e.g. a per-stage "sample"), and MergeOutput for results under the
nested "output" object.
*/
func (artifact *Artifact) Merge(key string, value any) {
	artifact.MergeFields(map[string]any{key: value})
}

/*
MergeFields writes several top-level key/value pairs into the artifact payload
with one payload mutation, preserving sibling keys already present.
*/
func (artifact *Artifact) MergeFields(values map[string]any) {
	if len(values) == 0 {
		return
	}

	body := artifact.payloadMap()

	for key, value := range values {
		body[key] = value
	}

	payload := body.Marshal()
	artifact.WithPayload(payload)
}

/*
MergeOutput writes a named result into the artifact payload's output object in
place, preserving sibling results written by earlier stages. The payload is the
data channel: input data and computation results both live here. Descriptors
(root, inputs, transforms) live on the attributes via Poke.
*/
func (artifact *Artifact) MergeOutput(key string, value any) {
	artifact.MergeOutputs(map[string]any{key: value})
}

/*
MergeOutputs writes several named results into the artifact payload's output
object with one payload mutation, preserving sibling results and top-level
payload data.
*/
func (artifact *Artifact) MergeOutputs(values map[string]any) {
	if len(values) == 0 {
		return
	}

	body := artifact.payloadMap()
	output, ok := body["output"].(map[string]any)

	if !ok {
		if typed, typedOk := body["output"].(Map[any]); typedOk {
			output = map[string]any(typed)
		} else {
			output = map[string]any{}
		}
	}

	for key, value := range values {
		output[key] = value
	}

	body["output"] = output

	payload := body.Marshal()
	artifact.WithPayload(payload)
}

func (artifact *Artifact) payloadMap() Map[any] {
	payload := artifact.DecryptPayload()
	body := Map[any]{}

	if sonic.Unmarshal(payload, &body) != nil {
		return Map[any]{}
	}

	return body
}
