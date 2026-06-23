package datura

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/ast"
	"github.com/theapemachine/errnie"
)

func Peek[T any](artifact *Artifact, path ...any) T {
	var (
		zero T
		root ast.Node
		ok   bool
	)

	if artifact == nil {
		return zero
	}

	missing := func(err error) bool {
		message := err.Error()

		return strings.Contains(message, "value not exists") ||
			strings.Contains(message, "Syntax error") ||
			strings.Contains(message, "no encrypted payload") ||
			strings.Contains(message, "encrypted payload unavailable") ||
			strings.Contains(message, "encrypted key unavailable") ||
			strings.Contains(message, "read traversal limit reached")
	}

	if len(path) > 0 && path[0] == "role" {
		role := errnie.Does(func() (string, error) {
			return artifact.Role()
		}).Or(func(err error) {
			errnie.Error(errnie.Err(errnie.Validation, err.Error(), err))
		}).Value()

		if role != "" {
			if zero, ok = any(role).(T); ok {
				return zero
			}
		}
	}

	if len(path) > 0 && path[0] == "scope" {
		scope := errnie.Does(func() (string, error) {
			return artifact.Scope()
		}).Or(func(err error) {
			errnie.Error(errnie.Err(errnie.Validation, err.Error(), err))
		}).Value()

		if scope != "" {
			if zero, ok = any(scope).(T); ok {
				return zero
			}
		}
	}

	if len(path) > 0 && path[0] == "origin" {
		origin := errnie.Does(func() (string, error) {
			return artifact.Origin()
		}).Or(func(err error) {
			errnie.Error(errnie.Err(errnie.Validation, err.Error(), err))
		}).Value()

		if origin != "" {
			if zero, ok = any(origin).(T); ok {
				return zero
			}
		}
	}

	if len(path) > 0 && path[0] == "destination" {
		destination := errnie.Does(func() (string, error) {
			return artifact.Destination()
		}).Or(func(err error) {
			errnie.Error(errnie.Err(errnie.Validation, err.Error(), err))
		}).Value()

		if destination != "" {
			if zero, ok = any(destination).(T); ok {
				return zero
			}
		}
	}

	for _, region := range []func() ([]byte, error){
		artifact.Attributes, artifact.decryptPayload,
	} {
		content, err := region()

		if err != nil {
			if missing(err) {
				continue
			}

			errnie.Error(errnie.Err(
				errnie.Validation, err.Error(), err,
			).With(artifact.Log()...))
			continue
		}

		content = bytes.TrimSpace(content)

		if len(content) == 0 {
			continue
		}

		if len(path) > 0 && content[0] != '{' && content[0] != '[' {
			continue
		}

		if !json.Valid(content) {
			continue
		}

		root = errnie.Does(func() (ast.Node, error) {
			return sonic.Get(content, path...)
		}).Or(func(err error) {
			if missing(err) {
				return
			}

			errnie.Error(errnie.Err(
				errnie.Validation, err.Error(), err,
			).With(artifact.Log()...))
		}).Value()

		if root.Exists() {
			break
		}
	}

	if !root.Exists() {
		return zero
	}

	value := errnie.Does(func() (any, error) {
		return root.Interface()
	}).Or(func(err error) {
		if missing(err) {
			return
		}

		errnie.Error(errnie.Err(errnie.Validation, err.Error(), err))
	}).Value()

	if zero, ok = value.(T); ok {
		return zero
	}

	if zero, ok = numericPeek[T](value); ok {
		return zero
	}

	return zero
}

func numericPeek[T any](value any) (T, bool) {
	var zero T

	source, ok := value.(float64)

	if !ok || math.IsNaN(source) || math.IsInf(source, 0) {
		return zero, false
	}

	if math.Trunc(source) != source {
		return zero, false
	}

	switch any(zero).(type) {
	case int:
		converted := int(source)

		if float64(converted) == source {
			return any(converted).(T), true
		}
	case int64:
		converted := int64(source)

		if float64(converted) == source {
			return any(converted).(T), true
		}
	}

	return zero, false
}

func (artifact *Artifact) Poke(value any, path ...any) *Artifact {
	root := errnie.Does(func() (ast.Node, error) {
		return sonic.Get(errnie.Does(func() ([]byte, error) {
			return artifact.Attributes()
		}).Or(func(err error) {
			errnie.Error(errnie.Err(errnie.Validation, "attribute peek failed", err))
		}).Value())
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

func finite(value any) any {
	switch typed := value.(type) {
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return 0.0
		}

		return typed
	case []float64:
		values := make([]float64, len(typed))

		for index, sample := range typed {
			values[index] = finite(sample).(float64)
		}

		return values
	case Map[float64]:
		values := Map[float64]{}

		for key, sample := range typed {
			values[key] = finite(sample).(float64)
		}

		return values
	}

	return value
}
