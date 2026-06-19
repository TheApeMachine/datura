package datura

import (
	"bytes"
	"math"

	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/ast"
	"github.com/theapemachine/errnie"
)

func Peek[T any](artifact *Artifact, path ...any) T {
	var zero T

	root, ok := ensureAttributesRoot(artifact)

	if !ok {
		return zero
	}

	state := artifactStreamStateFor(artifact)
	raw := attributeBytes(state, root)

	if len(raw) == 0 {
		return zero
	}

	node := errnie.Does(func() (ast.Node, error) {
		return sonic.Get(raw, path...)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.NotFound, "attribute path unavailable", err))
	}).Value()

	value := errnie.Does(func() (any, error) {
		return node.Interface()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.NotFound, "attribute value unavailable", err))
	}).Value()

	if typed, match := coerceAttributeValue[T](value); match {
		return typed
	}

	blob := errnie.Does(func() ([]byte, error) {
		return sonic.Marshal(value)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "attribute coercion marshal failed", err))
	}).Value()

	var typed T

	errnie.Error(sonic.Unmarshal(blob, &typed))

	return typed
}

func coerceAttributeValue[T any](value any) (T, bool) {
	var zero T

	if typed, match := value.(T); match {
		return typed, true
	}

	_, isFloat := any(zero).(float64)

	if !isFloat {
		return zero, false
	}

	stringValue, isString := value.(string)

	if !isString {
		return zero, false
	}

	switch stringValue {
	case "NaN":
		return any(math.NaN()).(T), true
	case "+Inf":
		return any(math.Inf(1)).(T), true
	case "-Inf":
		return any(math.Inf(-1)).(T), true
	default:
		return zero, false
	}
}

func encodeAttributeValue(value any) any {
	switch typed := value.(type) {
	case float64:
		return encodeFloat64(typed)
	case []float64:
		encoded := make([]any, len(typed))

		for index, sample := range typed {
			encoded[index] = encodeFloat64(sample)
		}

		return encoded
	case Map[float64]:
		encoded := make(map[string]any, len(typed))

		for key, sample := range typed {
			encoded[key] = encodeFloat64(sample)
		}

		return encoded
	case map[string]float64:
		encoded := make(map[string]any, len(typed))

		for key, sample := range typed {
			encoded[key] = encodeFloat64(sample)
		}

		return encoded
	default:
		return value
	}
}

func encodeFloat64(value float64) any {
	if math.IsNaN(value) {
		return "NaN"
	}

	if math.IsInf(value, 1) {
		return "+Inf"
	}

	if math.IsInf(value, -1) {
		return "-Inf"
	}

	return value
}

func (artifact *Artifact) Poke(value any, path ...any) *Artifact {
	if len(path) == 0 {
		return artifact
	}

	root, ok := ensureAttributesRoot(artifact)

	if !ok {
		return artifact
	}

	node := &root

	for index := 0; index < len(path)-1; index++ {
		switch segment := path[index].(type) {
		case string:
			if !node.Get(segment).Exists() {
				errnie.Does(func() (bool, error) {
					return node.Set(segment, ast.NewObject(nil))
				}).Or(func(err error) {
					errnie.Error(errnie.Err(errnie.Validation, "attribute path unavailable", err))
				})
			}

			node = node.Get(segment)
		case int:
			if !node.Index(segment).Exists() {
				length, _ := node.Len()

				for slot := length; slot <= segment; slot++ {
					errnie.Does(func() (struct{}, error) {
						return struct{}{}, node.Add(ast.NewNull())
					}).Or(func(err error) {
						errnie.Error(errnie.Err(errnie.Validation, "attribute path unavailable", err))
					})
				}
			}

			node = node.Index(segment)
		}
	}

	errnie.Does(func() (bool, error) {
		switch leaf := path[len(path)-1].(type) {
		case string:
			return node.SetAny(leaf, encodeAttributeValue(value))
		case int:
			return node.SetAnyByIndex(leaf, encodeAttributeValue(value))
		default:
			return false, errnie.Err(errnie.Validation, "attribute path segment must be int or string", nil)
		}
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "attribute poke failed", err))
	})

	state := artifactStreamStateFor(artifact)
	state.attributesRoot = root
	state.attributesDirty = true

	return artifact
}

func (artifact *Artifact) Clear(path ...any) *Artifact {
	if len(path) == 0 {
		return artifact
	}

	root, ok := ensureAttributesRoot(artifact)

	if !ok {
		return artifact
	}

	node := &root

	for index := 0; index < len(path)-1; index++ {
		switch segment := path[index].(type) {
		case string:
			next := node.Get(segment)

			if !next.Exists() {
				return artifact
			}

			node = next
		case int:
			next := node.Index(segment)

			if !next.Exists() {
				return artifact
			}

			node = next
		}
	}

	errnie.Does(func() (bool, error) {
		switch leaf := path[len(path)-1].(type) {
		case string:
			return node.Unset(leaf)
		case int:
			return node.UnsetByIndex(leaf)
		default:
			return false, errnie.Err(errnie.Validation, "attribute path segment must be int or string", nil)
		}
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "attribute clear failed", err))
	})

	state := artifactStreamStateFor(artifact)
	state.attributesRoot = root
	state.attributesDirty = true

	return artifact
}

func (artifact *Artifact) RetainStageAttributes() *Artifact {
	state := artifactStreamStateFor(artifact)
	state.retainStageAttributes = true

	return artifact
}

func ensureAttributesRoot(artifact *Artifact) (ast.Node, bool) {
	state := artifactStreamStateFor(artifact)

	raw := errnie.Does(func() ([]byte, error) {
		return artifact.Attributes()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.NotFound, "attributes unavailable", err))
	}).Value()

	if state.attributesParsed {
		if state.attributesDirty {
			return state.attributesRoot, true
		}

		if bytes.Equal(state.attributesBytes, raw) {
			return state.attributesRoot, true
		}
	}

	if len(raw) == 0 {
		state.attributesRoot = ast.NewObject(nil)
		state.attributesBytes = nil
		state.attributesParsed = true
		state.attributesDirty = false

		return state.attributesRoot, true
	}

	root := errnie.Does(func() (ast.Node, error) {
		return sonic.Get(raw)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.NotFound, "attributes unavailable", err))
	}).Value()

	errnie.Does(func() (struct{}, error) {
		return struct{}{}, root.Load()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "attributes unavailable", err))
	})

	state.attributesRoot = root
	state.attributesBytes = append([]byte(nil), raw...)
	state.attributesParsed = true
	state.attributesDirty = false

	return state.attributesRoot, true
}

func flushAttributesRoot(artifact *Artifact) {
	state := artifactStreamStateFor(artifact)

	if !state.attributesDirty || !state.attributesParsed {
		return
	}

	encoded := errnie.Does(func() ([]byte, error) {
		return state.attributesRoot.MarshalJSON()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "attributes marshal failed", err))
	}).Value()

	if len(encoded) == 0 {
		return
	}

	errnie.Error(artifact.SetAttributes(encoded))
	state.attributesBytes = append([]byte(nil), encoded...)
	state.attributesDirty = false
}

func mergeStageAttributes(artifact *Artifact, preserved ast.Node) error {
	state := artifactStreamStateFor(artifact)
	state.attributesParsed = false
	state.attributesDirty = false

	inbound, ok := ensureAttributesRoot(artifact)

	if !ok {
		return errnie.Err(errnie.Validation, "inbound attributes unavailable", nil)
	}

	if !preserved.Exists() {
		state.attributesRoot = inbound
		state.attributesDirty = true

		return nil
	}

	errnie.Does(func() (struct{}, error) {
		return struct{}{}, preserved.Load()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "preserved attributes unavailable", err))
	})

	iterator, err := preserved.Properties()

	if errnie.Error(err) != nil {
		return errnie.Err(errnie.Validation, "preserved attributes unavailable", err)
	}

	var pair ast.Pair

	for iterator.Next(&pair) {
		errnie.Does(func() (bool, error) {
			return inbound.Set(pair.Key, pair.Value)
		}).Or(func(setErr error) {
			errnie.Error(errnie.Err(errnie.Validation, "attribute merge failed", setErr))
		})
	}

	state.attributesRoot = inbound
	state.attributesDirty = true

	return nil
}

func attributeBytes(state *artifactStreamState, root ast.Node) []byte {
	if state.attributesDirty {
		return errnie.Does(func() ([]byte, error) {
			return root.MarshalJSON()
		}).Or(func(err error) {
			errnie.Error(errnie.Err(errnie.Validation, "attributes marshal failed", err))
		}).Value()
	}

	return state.attributesBytes
}
