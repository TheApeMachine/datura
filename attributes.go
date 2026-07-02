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
	var zero T

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

	if len(path) > 0 {
		var (
			meta string
			err  error
		)

		switch path[0] {
		case "role":
			meta, err = artifact.Role()
		case "scope":
			meta, err = artifact.Scope()
		case "origin":
			meta, err = artifact.Origin()
		case "destination":
			meta, err = artifact.Destination()
		}

		if err != nil {
			errnie.Error(errnie.Err(errnie.Validation, err.Error(), err))
		}

		if meta != "" {
			if typed, ok := any(meta).(T); ok {
				return typed
			}
		}
	}

	var value any
	found := false

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

		if !json.Valid(content) {
			continue
		}

		root, err := sonic.Get(content, path...)

		if err != nil {
			if missing(err) {
				continue
			}

			errnie.Error(errnie.Err(
				errnie.Validation, err.Error(), err,
			).With(artifact.Log()...))

			continue
		}

		if !root.Exists() {
			continue
		}

		value, err = root.Interface()

		if err != nil {
			if missing(err) {
				continue
			}

			errnie.Error(errnie.Err(errnie.Validation, err.Error(), err))

			continue
		}

		found = true

		break
	}

	if !found {
		return zero
	}

	if typed, ok := value.(T); ok {
		return typed
	}

	if typed, ok := numericPeek[T](value); ok {
		return typed
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
