package datura

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
	"time"

	"capnproto.org/go/capnp/v3"
	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
)

type Map map[string]any

func (m Map) Marshal() []byte {
	payload, err := sonic.Marshal(m)

	if err != nil {
		return nil
	}

	return payload
}

func (m Map) Unmarshal(payload []byte) error {
	return sonic.Unmarshal(payload, &m)
}

/*
MapPeek returns the typed value at key from m, returning the zero value when
absent or unparseable.
*/
func MapPeek[T any](m Map, key string) T {
	value, _ := MapPeekOK[T](m, key)

	return value
}

/*
MapPeekOK returns the typed value at key from m and whether conversion succeeded.
*/
func MapPeekOK[T any](m Map, key string) (T, bool) {
	var zero T

	raw, ok := m[key]

	if !ok {
		return zero, false
	}

	return peekFromValue[T](raw)
}

/*
Peek returns the typed attribute value from artifact, returning the zero value
when absent or unparseable.
*/
func Peek[T any](artifact *Artifact, key string) T {
	value, _ := PeekOK[T](artifact, key)

	return value
}

/*
PeekOK returns the typed attribute value from artifact and whether conversion
succeeded.
*/
func PeekOK[T any](artifact *Artifact, key string) (T, bool) {
	var zero T

	if artifact == nil {
		return zero, false
	}

	return peekFromString[T](artifact.Peek(key))
}

func peekFromString[T any](raw string) (T, bool) {
	var zero T

	if raw == "" {
		return zero, false
	}

	switch any(zero).(type) {
	case string:
		return any(raw).(T), true
	case float64:
		value, err := strconv.ParseFloat(raw, 64)

		if err != nil {
			return zero, false
		}

		return any(value).(T), true
	case float32:
		value, err := strconv.ParseFloat(raw, 32)

		if err != nil {
			return zero, false
		}

		return any(float32(value)).(T), true
	case int:
		value, err := strconv.Atoi(raw)

		if err != nil {
			return zero, false
		}

		return any(value).(T), true
	case int64:
		value, err := strconv.ParseInt(raw, 10, 64)

		if err != nil {
			return zero, false
		}

		return any(value).(T), true
	case bool:
		value, err := strconv.ParseBool(raw)

		if err != nil {
			return zero, false
		}

		return any(value).(T), true
	case time.Time:
		parsed, err := time.Parse(time.RFC3339Nano, raw)

		if err != nil {
			return zero, false
		}

		return any(parsed).(T), true
	case []float64:
		values, ok := parseFloatList(raw)

		if !ok {
			return zero, false
		}

		return any(values).(T), true
	default:
		return zero, false
	}
}

func peekFromValue[T any](raw any) (T, bool) {
	var zero T

	switch value := raw.(type) {
	case T:
		return value, true
	case string:
		return peekFromString[T](value)
	case float64:
		switch any(zero).(type) {
		case float64:
			return any(value).(T), true
		case float32:
			return any(float32(value)).(T), true
		case int:
			return any(int(value)).(T), true
		case int64:
			return any(int64(value)).(T), true
		}
	case float32:
		switch any(zero).(type) {
		case float64:
			return any(float64(value)).(T), true
		case float32:
			return any(value).(T), true
		case int:
			return any(int(value)).(T), true
		case int64:
			return any(int64(value)).(T), true
		}
	case int:
		switch any(zero).(type) {
		case float64:
			return any(float64(value)).(T), true
		case float32:
			return any(float32(value)).(T), true
		case int:
			return any(value).(T), true
		case int64:
			return any(int64(value)).(T), true
		}
	case int64:
		switch any(zero).(type) {
		case float64:
			return any(float64(value)).(T), true
		case float32:
			return any(float32(value)).(T), true
		case int:
			return any(int(value)).(T), true
		case int64:
			return any(value).(T), true
		}
	case time.Time:
		switch any(zero).(type) {
		case time.Time:
			return any(value).(T), true
		case string:
			return any(value.Format(time.RFC3339Nano)).(T), true
		}
	}

	return zero, false
}

func parseFloatList(raw string) ([]float64, bool) {
	parts := strings.Split(raw, ",")
	values := make([]float64, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)

		if part == "" {
			continue
		}

		value, err := strconv.ParseFloat(part, 64)

		if err != nil {
			return nil, false
		}

		values = append(values, value)
	}

	if len(values) == 0 {
		return nil, false
	}

	return values, true
}

func As[T any](artifact *Artifact) T {
	var zero T

	payload, err := artifact.Payload()

	if err != nil {
		return zero
	}

	if err := sonic.Unmarshal(payload, &zero); err != nil {
		return zero
	}

	return zero
}

/*
Encode encodes the artifact to a byte slice.
*/
func (artifact *Artifact) Encode(buf []byte) {
	encoder := capnp.NewEncoder(bytes.NewBuffer(buf))

	if err := encoder.Encode(artifact.Message()); err != nil {
		errnie.Error(err)
	}
}

/*
Decode decodes the artifact from a byte slice.
*/
func (artifact *Artifact) Decode(buf []byte) *Artifact {
	var (
		err     error
		msg     *capnp.Message
		artfct  Artifact
		decoder = capnp.NewDecoder(bytes.NewBuffer(buf))
	)

	if msg, err = decoder.Decode(); err != nil {
		errnie.Error(err)
		return nil
	}

	if artfct, err = ReadRootArtifact(msg); err != nil {
		errnie.Error(err)
		return nil
	}

	*artifact = artfct
	return artifact
}

/*
Marshal converts the artifact to a byte slice.
*/
func (artifact *Artifact) Marshal() []byte {
	var (
		buf []byte
		err error
	)

	if buf, err = artifact.Message().Marshal(); err != nil {
		errnie.Error(err)
	}

	return buf
}

/*
Unmarshal converts a byte slice to an artifact.
*/
func (artifact *Artifact) Unmarshal(buf []byte) *Artifact {
	var (
		msg    *capnp.Message
		artfct Artifact
		err    error
	)

	if len(buf) == 0 {
		errnie.Error(errors.New("empty buffer"))
		return nil
	}

	if msg, err = capnp.Unmarshal(buf); err != nil {
		errnie.Error(err)
		return nil
	}

	if msg == nil {
		errnie.Error(errors.New("nil message after unmarshal"))
		return nil
	}

	if artfct, err = ReadRootArtifact(msg); err != nil {
		errnie.Error(err)
		return nil
	}

	*artifact = artfct
	return artifact
}

func (artifact *Artifact) Pack() []byte {
	var (
		buf []byte
		err error
	)

	if buf, err = artifact.Message().MarshalPacked(); err != nil {
		errnie.Error(err)
	}

	return buf
}

/*
Unpack converts a byte slice to an artifact.
*/
func (artifact *Artifact) Unpack(buf []byte) *Artifact {
	var (
		msg    *capnp.Message
		artfct Artifact
		err    error
	)

	if len(buf) == 0 {
		errnie.Error(errors.New("empty buffer"))
		return nil
	}

	if msg, err = capnp.UnmarshalPacked(buf); err != nil {
		errnie.Error(err)
		return nil
	}

	if msg == nil {
		errnie.Error(errors.New("nil message after unmarshal"))
		return nil
	}

	if artfct, err = ReadRootArtifact(msg); err != nil {
		errnie.Error(err)
		return nil
	}

	*artifact = artfct
	return artifact
}
