package datura

import (
	"math"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/ast"
	"github.com/theapemachine/errnie"
)

func Peek[T any](artifact *Artifact, path ...any) T {
	var zero T

	attributeNode := artifact.attributesRoot().GetByPath(path...)

	if attributeNode != nil && attributeNode.Exists() {
		return peekNodeValue[T](attributeNode)
	}

	payload, err := artifact.decryptPayload()

	if err != nil || len(payload) == 0 || !payloadLooksJSON(payload) {
		return zero
	}

	payloadNode := errnie.Does(func() (ast.Node, error) {
		return sonic.Get(payload, path...)
	}).Or(func(err error) {
		if strings.Contains(err.Error(), "value not exists") {
			return
		}

		errnie.Error(errnie.Err(
			errnie.Validation, err.Error(), err,
		).With(artifact.Log()...))
	}).Value()

	if !payloadNode.Exists() {
		return zero
	}

	return peekNodeValue[T](&payloadNode)
}

func payloadLooksJSON(payload []byte) bool {
	for len(payload) > 0 {
		switch payload[0] {
		case ' ', '\n', '\t', '\r':
			payload = payload[1:]
		default:
			switch payload[0] {
			case '{', '[', '"', '-':
				return true
			case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
				return true
			case 't', 'f', 'n':
				return true
			default:
				return false
			}
		}
	}

	return false
}

func peekNodeValue[T any](node *ast.Node) T {
	var zero T

	raw := errnie.Does(func() (any, error) {
		return node.Interface()
	}).Or(func(err error) {
		if strings.Contains(err.Error(), "value not exists") {
			return
		}

		errnie.Error(errnie.Err(errnie.Validation, err.Error(), err))
	}).Value()

	if raw == nil {
		return zero
	}

	if typed, matches := raw.(T); matches {
		return typed
	}

	switch target := any(zero).(type) {
	case int:
		if value, numeric := raw.(float64); numeric {
			return any(int(value)).(T)
		}
	case int64:
		if value, numeric := raw.(float64); numeric {
			return any(int64(value)).(T)
		}
	case uint:
		if value, numeric := raw.(float64); numeric && value >= 0 {
			return any(uint(value)).(T)
		}
	case uint64:
		if value, numeric := raw.(float64); numeric && value >= 0 {
			return any(uint64(value)).(T)
		}
	case float32:
		if value, numeric := raw.(float64); numeric {
			return any(float32(value)).(T)
		}
	default:
		_ = target
	}

	return zero
}

func (artifact *Artifact) Poke(value any, path ...any) *Artifact {
	entry := attributesCacheEntryFor(artifact)

	entry.root.SetAnyByPath(sanitizePokeValue(value), path...)
	rematerializeAttributes(entry)
	entry.dirty = true

	return artifact
}

func sanitizePokeValue(value any) any {
	switch typed := value.(type) {
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return float64(0)
		}

		return typed
	case float32:
		if math.IsNaN(float64(typed)) || math.IsInf(float64(typed), 0) {
			return float32(0)
		}

		return typed
	case []float64:
		sanitized := make([]float64, len(typed))

		for index, sample := range typed {
			if math.IsNaN(sample) || math.IsInf(sample, 0) {
				sanitized[index] = 0
				continue
			}

			sanitized[index] = sample
		}

		return sanitized
	case []float32:
		sanitized := make([]float32, len(typed))

		for index, sample := range typed {
			if math.IsNaN(float64(sample)) || math.IsInf(float64(sample), 0) {
				sanitized[index] = 0
				continue
			}

			sanitized[index] = sample
		}

		return sanitized
	case Map[float64]:
		sanitized := make(map[string]float64, len(typed))

		for key, sample := range typed {
			if math.IsNaN(sample) || math.IsInf(sample, 0) {
				sanitized[key] = 0
				continue
			}

			sanitized[key] = sample
		}

		return sanitized
	case map[string]float64:
		sanitized := make(map[string]float64, len(typed))

		for key, sample := range typed {
			if math.IsNaN(sample) || math.IsInf(sample, 0) {
				sanitized[key] = 0
				continue
			}

			sanitized[key] = sample
		}

		return sanitized
	case Map[any]:
		sanitized := make(map[string]any, len(typed))

		for key, nested := range typed {
			sanitized[key] = sanitizePokeValue(nested)
		}

		return sanitized
	case map[string]any:
		sanitized := make(map[string]any, len(typed))

		for key, nested := range typed {
			sanitized[key] = sanitizePokeValue(nested)
		}

		return sanitized
	case []any:
		sanitized := make([]any, len(typed))

		for index, nested := range typed {
			sanitized[index] = sanitizePokeValue(nested)
		}

		return sanitized
	}

	return value
}
