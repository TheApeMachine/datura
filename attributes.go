package datura

import (
	"bytes"
	"fmt"
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
			strings.Contains(message, "payload unavailable") ||
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

	if typed, ok := slicePeek[T](value); ok {
		return typed
	}

	return zero
}

func LookupAttribute[T any](artifact *Artifact, path ...any) (T, bool, error) {
	return lookupRegion[T](artifact, "attributes", path...)
}

func LookupPayload[T any](artifact *Artifact, path ...any) (T, bool, error) {
	return lookupRegion[T](artifact, "payload", path...)
}

func lookupRegion[T any](
	artifact *Artifact,
	region string,
	path ...any,
) (T, bool, error) {
	var zero T

	if artifact == nil {
		return zero, false, errnie.Err(errnie.Validation, "datura: nil artifact", nil)
	}

	content, err := lookupRegionBytes(artifact, region)
	if err != nil {
		if lookupMissing(err) {
			return zero, false, nil
		}

		return zero, false, err
	}

	content = bytes.TrimSpace(content)
	if len(content) == 0 {
		return zero, false, nil
	}

	root, err := sonic.Get(content, path...)
	if err != nil {
		if lookupMissing(err) {
			return zero, false, nil
		}

		return zero, false, err
	}

	if !root.Exists() {
		return zero, false, nil
	}

	value, err := root.Interface()
	if err != nil {
		return zero, false, err
	}

	if typed, ok := convertLookup[T](value); ok {
		return typed, true, nil
	}

	return zero, false, errnie.Err(
		errnie.Validation,
		fmt.Sprintf("datura: %s path has unexpected type", region),
		nil,
	)
}

func lookupRegionBytes(artifact *Artifact, region string) ([]byte, error) {
	switch region {
	case "attributes":
		return artifact.Attributes()
	case "payload":
		return artifact.decryptPayload()
	default:
		return nil, errnie.Err(errnie.Validation, "datura: unknown lookup region", nil)
	}
}

func lookupMissing(err error) bool {
	if err == nil {
		return false
	}

	message := err.Error()

	return strings.Contains(message, "value not exists") ||
		strings.Contains(message, "no encrypted payload") ||
		strings.Contains(message, "payload unavailable") ||
		strings.Contains(message, "encrypted payload unavailable") ||
		strings.Contains(message, "encrypted key unavailable") ||
		strings.Contains(message, "read traversal limit reached")
}

func convertLookup[T any](value any) (T, bool) {
	if typed, ok := value.(T); ok {
		return typed, true
	}

	if typed, ok := numericPeek[T](value); ok {
		return typed, true
	}

	return slicePeek[T](value)
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

func slicePeek[T any](value any) (T, bool) {
	var zero T

	items, ok := value.([]any)

	if !ok {
		return zero, false
	}

	switch any(zero).(type) {
	case []string:
		values := make([]string, len(items))

		for index, item := range items {
			typed, ok := item.(string)

			if !ok {
				return zero, false
			}

			values[index] = typed
		}

		return any(values).(T), true
	case []float64:
		values := make([]float64, len(items))

		for index, item := range items {
			switch typed := item.(type) {
			case float64:
				values[index] = typed
			case int:
				values[index] = float64(typed)
			default:
				return zero, false
			}
		}

		return any(values).(T), true
	}

	return zero, false
}

func (artifact *Artifact) Poke(value any, path ...any) *Artifact {
	var root ast.Node

	attributes, err := artifact.Attributes()

	if err == nil {
		attributes = bytes.TrimSpace(attributes)
	}

	if err != nil || len(attributes) == 0 {
		root = ast.NewObject(nil)
	} else if parsed, parseErr := sonic.Get(attributes); parseErr == nil {
		root = parsed
	} else {
		errnie.Error(errnie.Err(errnie.Validation, "attribute peek failed", parseErr))
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
