package datura

import (
	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/ast"
	"github.com/theapemachine/errnie"
)

func attributeBytes(artifact *Artifact) []byte {
	return errnie.Does(func() ([]byte, error) {
		return artifact.Attributes()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "attributes unavailable", err))
	}).Value()
}

func attributeRoot(artifact *Artifact) ast.Node {
	encoded := attributeBytes(artifact)

	if len(encoded) > 0 {
		root := errnie.Does(func() (ast.Node, error) {
			return sonic.Get(encoded)
		}).Or(func(err error) {
			errnie.Error(errnie.Err(errnie.Validation, "attributes parse failed", err))
		}).Value()

		if root.Exists() {
			return root
		}
	}

	payload, err := artifact.decryptPayload()

	if err != nil || len(payload) == 0 {
		return ast.Node{}
	}

	root, err := sonic.Get(payload)

	if err != nil || !root.Exists() {
		if err != nil {
			errnie.Error(errnie.Err(errnie.Validation, "payload parse failed", err))
		}

		return ast.Node{}
	}

	return root
}

func mutableAttributeRoot(artifact *Artifact) ast.Node {
	encoded := attributeBytes(artifact)

	if len(encoded) > 0 {
		root := errnie.Does(func() (ast.Node, error) {
			return sonic.Get(encoded)
		}).Or(func(err error) {
			errnie.Error(errnie.Err(errnie.Validation, "attributes parse failed", err))
		}).Value()

		if root.Exists() {
			return root
		}
	}

	return ast.NewObject(nil)
}

func Peek[T any](artifact *Artifact, path ...any) T {
	var zero T

	root := attributeRoot(artifact)

	if !root.Exists() {
		return zero
	}

	node := root.GetByPath(path...)

	if !node.Exists() {
		return zero
	}

	value, err := node.Interface()

	if err != nil {
		errnie.Error(errnie.Err(errnie.Validation, "attribute peek failed", err))

		return zero
	}

	typed, ok := value.(T)

	if !ok {
		// sonic decodes JSON arrays as []any and JSON numbers as float64, so a
		// direct assertion to a concrete typed slice or numeric type fails even
		// when the stored data is sound; coerce into the requested type so callers
		// Peek the same shapes they Poke.
		if coerced, coercedOK := coerceTyped[T](value); coercedOK {
			return coerced
		}

		errnie.Error(errnie.Err(errnie.Validation, "attribute peek type mismatch", nil))

		return zero
	}

	return typed
}

/*
coerceTyped converts sonic's generic decode ([]any of float64/string, or a bare
float64) into the concrete slice or numeric type a caller requested via Peek[T].
*/
func coerceTyped[T any](value any) (T, bool) {
	var zero T

	switch any(zero).(type) {
	case []string:
		elements, ok := value.([]any)

		if !ok {
			return zero, false
		}

		out := make([]string, len(elements))

		for index, element := range elements {
			str, strOK := element.(string)

			if !strOK {
				return zero, false
			}

			out[index] = str
		}

		typed, _ := any(out).(T)

		return typed, true
	case []float64:
		elements, ok := value.([]any)

		if !ok {
			return zero, false
		}

		out := make([]float64, len(elements))

		for index, element := range elements {
			number, numberOK := element.(float64)

			if !numberOK {
				return zero, false
			}

			out[index] = number
		}

		typed, _ := any(out).(T)

		return typed, true
	case int:
		number, ok := value.(float64)

		if !ok {
			return zero, false
		}

		typed, _ := any(int(number)).(T)

		return typed, true
	case int64:
		number, ok := value.(float64)

		if !ok {
			return zero, false
		}

		typed, _ := any(int64(number)).(T)

		return typed, true
	}

	return zero, false
}

func (artifact *Artifact) Poke(value any, path ...any) *Artifact {
	root := mutableAttributeRoot(artifact)

	errnie.Does(func() (bool, error) {
		return (&root).SetAnyByPath(value, path...)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "attribute poke failed", err))
	}).Value()

	encoded := errnie.Does(func() ([]byte, error) {
		return root.MarshalJSON()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "attributes marshal failed", err))
	}).Value()

	errnie.Error(artifact.SetAttributes(encoded))

	return artifact
}
