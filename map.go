package datura

import (
	"encoding/json"

	"github.com/bytedance/sonic"
)

type Map[T any] map[string]T

func (m Map[T]) Marshal() []byte {
	payload, err := sonic.Marshal(m)

	if err != nil {
		return nil
	}

	return payload
}

/*
Merge writes a key/value into the artifact payload in place, preserving sibling
keys already present. The payload is the data channel; use this for top-level
payload data (e.g. a per-stage "sample"), and MergeOutput for results under the
nested "output" object.
*/
func (artifact *Artifact) Merge(key string, value any) {
	body := artifact.payloadMap()
	body[key] = value
	artifact.WithPayload(body.Marshal())
}

/*
MergeOutput writes a named result into the artifact payload's output object in
place, preserving sibling results written by earlier stages. The payload is the
data channel: input data and computation results both live here. Descriptors
(root, inputs, transforms) live on the attributes via Poke.
*/
func (artifact *Artifact) MergeOutput(key string, value any) {
	body := artifact.payloadMap()
	output, ok := body["output"].(map[string]any)

	if !ok {
		if typed, typedOk := body["output"].(Map[any]); typedOk {
			output = map[string]any(typed)
		} else {
			output = map[string]any{}
		}
	}

	output[key] = value
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
