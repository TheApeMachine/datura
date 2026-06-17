package datura

import (
	"fmt"
	"strconv"

	"github.com/theapemachine/errnie"
)

func peekArtifactField[T any](artifact *Artifact, key string) (T, bool) {
	var zero T

	if artifact == nil {
		return zero, false
	}

	var fieldValue string

	switch key {
	case "role":
		role, roleErr := artifact.Role()

		if roleErr != nil || role == "" {
			return zero, false
		}

		fieldValue = role
	case "scope":
		scope, scopeErr := artifact.Scope()

		if scopeErr != nil || scope == "" {
			return zero, false
		}

		fieldValue = scope
	case "destination":
		destination, destinationErr := artifact.Destination()

		if destinationErr != nil || destination == "" {
			return zero, false
		}

		fieldValue = destination
	case "origin":
		origin, originErr := artifact.Origin()

		if originErr != nil || origin == "" {
			return zero, false
		}

		fieldValue = origin
	default:
		return zero, false
	}

	typedValue, ok := any(fieldValue).(T)

	return typedValue, ok
}

func indexAttributeValue(state *artifactStreamState, key string, value Artifact_Attribute_value) {
	switch value.Which() {
	case Artifact_Attribute_value_Which_textValue:
		text, textErr := value.TextValue()

		if textErr != nil {
			return
		}

		state.cache[key] = text
	case Artifact_Attribute_value_Which_intValue:
		state.cache[key] = value.IntValue()
	case Artifact_Attribute_value_Which_floatValue:
		state.cache[key] = value.FloatValue()
	case Artifact_Attribute_value_Which_boolValue:
		state.cache[key] = value.BoolValue()
	case Artifact_Attribute_value_Which_binaryValue:
		binary, binaryErr := value.BinaryValue()

		if binaryErr != nil {
			return
		}

		state.cache[key] = binary
	}
}

func ensureArtifactIndex(artifact *Artifact) {
	state := artifactStreamStateFor(artifact)

	if state.indexed {
		return
	}

	state.indexed = true

	metadata, err := artifact.Attributes()

	if errnie.Error(err) != nil {
		return
	}

	for idx := range metadata.Len() {
		meta := metadata.At(idx)
		key, keyErr := meta.Key()

		if keyErr != nil {
			continue
		}

		indexAttributeValue(state, key, meta.Value())
	}
}

func peekFromCache[T any](state *artifactStreamState, key string) (T, bool) {
	var zero T

	rawVal, found := state.cache[key]

	if !found {
		return zero, false
	}

	if typedVal, ok := rawVal.(T); ok {
		return typedVal, true
	}

	switch any(zero).(type) {
	case int:
		if intVal, ok := rawVal.(int64); ok {
			return any(int(intVal)).(T), true
		}

		if strVal, ok := rawVal.(string); ok {
			if parsedInt, parseErr := strconv.Atoi(strVal); parseErr == nil {
				return any(parsedInt).(T), true
			}
		}
	case float64:
		if floatVal, ok := rawVal.(float64); ok {
			return any(floatVal).(T), true
		}

		if strVal, ok := rawVal.(string); ok {
			if parsedFloat, parseErr := strconv.ParseFloat(strVal, 64); parseErr == nil {
				return any(parsedFloat).(T), true
			}
		}
	case bool:
		if boolVal, ok := rawVal.(bool); ok {
			return any(boolVal).(T), true
		}

		if strVal, ok := rawVal.(string); ok {
			if parsedBool, parseErr := strconv.ParseBool(strVal); parseErr == nil {
				return any(parsedBool).(T), true
			}
		}
	}

	return zero, false
}

func syncArtifactCacheEntry(artifact *Artifact, key string, val any) {
	state := artifactStreamStateFor(artifact)

	if !state.indexed {
		return
	}

	state.cache[key] = val
}

func syncArtifactCacheBatch(artifact *Artifact, kv map[string]any) {
	state := artifactStreamStateFor(artifact)

	if !state.indexed {
		return
	}

	for key, val := range kv {
		state.cache[key] = val
	}
}

/*
PeekOK returns a typed metadata value and whether key was present.
The first lookup lazily indexes Cap'n Proto attributes into an O(1) cache.
*/
func PeekOK[T any](artifact *Artifact, key string) (T, bool) {
	var zero T

	if artifact == nil {
		return zero, false
	}

	ensureArtifactIndex(artifact)

	state := artifactStreamStateFor(artifact)

	if value, ok := peekFromCache[T](state, key); ok {
		return value, true
	}

	if value, ok := peekArtifactField[T](artifact, key); ok {
		return value, true
	}

	return zero, false
}

// GetMetaValue retrieves a typed metadata value from an artifact.
func Peek[T any](artifact *Artifact, key string) T {
	val, _ := PeekOK[T](artifact, key)

	return val
}

/*
AttributeValueString formats a capnp attribute union as a Poke-compatible string.
*/
func AttributeValueString(value Artifact_Attribute_value) (string, bool) {
	switch value.Which() {
	case Artifact_Attribute_value_Which_textValue:
		text, textErr := value.TextValue()

		if textErr != nil {
			return "", false
		}

		return text, true
	case Artifact_Attribute_value_Which_intValue:
		return strconv.FormatInt(value.IntValue(), 10), true
	case Artifact_Attribute_value_Which_floatValue:
		return strconv.FormatFloat(value.FloatValue(), 'g', -1, 64), true
	case Artifact_Attribute_value_Which_boolValue:
		return strconv.FormatBool(value.BoolValue()), true
	default:
		return "", false
	}
}

// SetMetaValue sets a metadata value with the appropriate type.
func (artifact *Artifact) SetMetaValue(key string, val any) error {
	return artifact.SetMetaValues(map[string]any{key: val})
}

/*
SetMetaValues updates or appends multiple metadata values in a single allocation pass.
*/
func (artifact *Artifact) SetMetaValues(kv map[string]any) error {
	if len(kv) == 0 {
		return nil
	}

	mdList, err := artifact.Attributes()

	if err != nil {
		return errnie.Error(err)
	}

	newMdList, err := artifact.NewAttributes(int32(mdList.Len() + len(kv)))

	if err != nil {
		return errnie.Error(err)
	}

	for idx := range mdList.Len() {
		if err = newMdList.Set(idx, mdList.At(idx)); err != nil {
			return errnie.Error(err)
		}
	}

	offset := mdList.Len()

	for key, val := range kv {
		item := newMdList.At(offset)

		if err = item.SetKey(key); err != nil {
			return errnie.Error(err)
		}

		switch typedValue := val.(type) {
		case string:
			item.Value().SetTextValue(typedValue)
		case int:
			item.Value().SetIntValue(int64(typedValue))
		case int64:
			item.Value().SetIntValue(typedValue)
		case float64:
			item.Value().SetFloatValue(typedValue)
		case bool:
			item.Value().SetBoolValue(typedValue)
		case []byte:
			item.Value().SetBinaryValue(typedValue)
		default:
			item.Value().SetTextValue(fmt.Sprintf("%v", typedValue))
		}

		offset++
	}

	syncArtifactCacheBatch(artifact, kv)

	return nil
}

/*
PeekEach walks the attributes list exactly once, delivering keys and values to a callback.
Return false from the callback to break the loop early.
*/
func (artifact *Artifact) PeekEach(fn func(key string, val Artifact_Attribute_value) bool) {
	if artifact == nil {
		return
	}

	metadata, err := artifact.Attributes()

	if errnie.Error(err) != nil {
		return
	}

	for idx := range metadata.Len() {
		meta := metadata.At(idx)
		key, keyErr := meta.Key()

		if keyErr != nil {
			continue
		}

		if !fn(key, meta.Value()) {
			break
		}
	}
}
