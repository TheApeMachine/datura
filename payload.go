package datura

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/ast"
	"github.com/theapemachine/errnie"
)

/*
PeekPayload returns a typed payload value or the type zero when absent.
Path segments use dot notation with numeric indices for arrays, e.g. "0.price",
or separate path arguments, e.g. "data", "price", 0.
*/
func PeekPayload[T any](artifact *Artifact, path ...any) T {
	var zero T

	node, found := payloadNodeAt(artifact, path...)

	if !found {
		return zero
	}

	value, ok := payloadNodeAs[T](*node)

	if ok {
		return value
	}

	raw := errnie.Does(func() (any, error) {
		return node.Interface()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.NotFound, "payload value unavailable", err))
	}).Value()

	if typed, match := raw.(T); match {
		return typed
	}

	blob := errnie.Does(func() ([]byte, error) {
		return sonic.Marshal(raw)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "payload coercion marshal failed", err))
	}).Value()

	var typed T

	errnie.Error(sonic.Unmarshal(blob, &typed))

	return typed
}

/*
PeekPayloadOK returns a typed payload value and reports whether the path existed.
*/
func PeekPayloadOK[T any](artifact *Artifact, path ...any) (T, bool) {
	var zero T

	node, found := payloadNodeAt(artifact, path...)

	if !found {
		return zero, false
	}

	value, ok := payloadNodeAs[T](*node)

	if ok {
		return value, true
	}

	raw := errnie.Does(func() (any, error) {
		return node.Interface()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.NotFound, "payload value unavailable", err))
	}).Value()

	if typed, match := raw.(T); match {
		return typed, true
	}

	blob := errnie.Does(func() ([]byte, error) {
		return sonic.Marshal(raw)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "payload coercion marshal failed", err))
	}).Value()

	var typed T

	if err := sonic.Unmarshal(blob, &typed); err != nil {
		return zero, false
	}

	return typed, true
}

func ensurePayloadRoot(artifact *Artifact) (ast.Node, bool) {
	payload := errnie.Does(func() ([]byte, error) {
		return artifact.DecryptPayload()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.NotFound, "payload unavailable", err))
	}).Value()

	if len(payload) == 0 {
		return ast.Node{}, false
	}

	state := artifactStreamStateFor(artifact)

	if state.payloadParsed && bytes.Equal(state.payloadBytes, payload) {
		return state.payloadRoot, true
	}

	root := errnie.Does(func() (ast.Node, error) {
		return sonic.Get(payload)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.NotFound, "payload unavailable", err))
	}).Value()

	state.payloadBytes = append([]byte(nil), payload...)
	state.payloadRoot = root
	state.payloadParsed = true

	return root, true
}

func payloadNodeAt(artifact *Artifact, path ...any) (*ast.Node, bool) {
	root, rootOK := ensurePayloadRoot(artifact)

	if !rootOK {
		return nil, false
	}

	segments := payloadPathSegments(path...)
	current := &root

	for _, segment := range segments {
		switch key := segment.(type) {
		case string:
			if key == "" {
				return nil, false
			}

			next := current.Get(key)

			if !next.Exists() {
				return nil, false
			}

			current = next
		case int:
			next := current.Index(key)

			if !next.Exists() {
				return nil, false
			}

			current = next
		default:
			return nil, false
		}
	}

	return current, true
}

func payloadPathSegments(path ...any) []any {
	if len(path) != 1 {
		return path
	}

	dotted, ok := path[0].(string)

	if !ok || !strings.Contains(dotted, ".") {
		return path
	}

	segments := strings.Split(dotted, ".")
	out := make([]any, len(segments))

	for index, segment := range segments {
		if numeric, parseErr := strconv.Atoi(segment); parseErr == nil {
			out[index] = numeric

			continue
		}

		out[index] = segment
	}

	return out
}

func payloadNodeAs[T any](node ast.Node) (T, bool) {
	var zero T

	switch any(zero).(type) {
	case string:
		value, err := node.String()

		if err != nil {
			return zero, false
		}

		return any(value).(T), true
	case float64:
		value, err := node.Float64()

		if err != nil {
			return zero, false
		}

		return any(value).(T), true
	case int:
		value, err := node.Int64()

		if err != nil {
			return zero, false
		}

		return any(int(value)).(T), true
	case int64:
		value, err := node.Int64()

		if err != nil {
			return zero, false
		}

		return any(value).(T), true
	case bool:
		value, err := node.Bool()

		if err != nil {
			return zero, false
		}

		return any(value).(T), true
	default:
		return zero, false
	}
}
