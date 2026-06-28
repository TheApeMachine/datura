package datura

import (
	"encoding/json"

	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/ast"
	"github.com/theapemachine/errnie"
)

type Map[T any] map[string]T

func (m Map[T]) Marshal() []byte {
	payload, err := sonic.Marshal(m)

	if err != nil {
		return nil
	}

	return payload
}

func (artifact *Artifact) PokePayload(value any, path ...any) *Artifact {
	root := errnie.Does(func() (ast.Node, error) {
		return sonic.Get(artifact.DecryptPayload())
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "attribute peek failed", err))
	}).Value()

	if !root.Exists() {
		root = ast.NewObject(nil)
	}

	root.SetAnyByPath(finite(value), path...)

	errnie.Error(artifact.SetAttributes(errnie.Does(func() ([]byte, error) {
		return root.MarshalJSON()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "attributes marshal failed", err))
	}).Value()))

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

	artifact.WithPayload(body.Marshal())
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
	artifact.WithPayload(body.Marshal())
}

func (artifact *Artifact) payloadMap() Map[any] {
	payload := artifact.DecryptPayload()

	if !json.Valid(payload) {
		return Map[any]{}
	}

	body := Map[any]{}

	if sonic.Unmarshal(payload, &body) != nil {
		return Map[any]{}
	}

	return body
}
