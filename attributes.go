package datura

import (
	"fmt"
	"strconv"

	"github.com/theapemachine/errnie"
)

// GetMetaValue retrieves a typed metadata value from an artifact
func Peek[T any](artifact *Artifact, key string) T {
	errnie.Debug("datura.GetMetaValue", "key", key)

	var (
		metadata Artifact_Attribute_List
		err      error
		result   T
	)

	if metadata, err = artifact.Attributes(); errnie.Error(err) != nil {
		return *new(T)
	}

	for idx := range metadata.Len() {
		var (
			k    string
			meta = metadata.At(idx)
		)

		if k, err = meta.Key(); errnie.Error(err) != nil {
			return *new(T)
		}

		if k == key {
			which := meta.Value().Which()

			// Use type assertion to determine the return type expected
			switch any(result).(type) {
			case string:
				// Handle string type
				if which == Artifact_Attribute_value_Which_textValue {
					val, _ := meta.Value().TextValue()
					return any(val).(T)
				}
			case int:
				// Handle integer type
				switch which {
				case Artifact_Attribute_value_Which_intValue:
					val := meta.Value().IntValue()
					return any(int(val)).(T)
				case Artifact_Attribute_value_Which_textValue:
					// Try to convert from string
					val, _ := meta.Value().TextValue()
					if i, err := strconv.Atoi(val); err == nil {
						return any(i).(T)
					}
				}
			case float64:
				// Handle float type
				switch which {
				case Artifact_Attribute_value_Which_floatValue:
					val := meta.Value().FloatValue()
					return any(val).(T)
				case Artifact_Attribute_value_Which_textValue:
					// Try to convert from string
					val, _ := meta.Value().TextValue()
					if f, err := strconv.ParseFloat(val, 64); err == nil {
						return any(f).(T)
					}
				}
			case bool:
				// Handle boolean type
				switch which {
				case Artifact_Attribute_value_Which_boolValue:
					val := meta.Value().BoolValue()
					return any(val).(T)
				case Artifact_Attribute_value_Which_textValue:
					// Try to convert from string
					val, _ := meta.Value().TextValue()
					if b, err := strconv.ParseBool(val); err == nil {
						return any(b).(T)
					}
				}
			}
		}
	}

	return *new(T)
}

/*
PeekOK returns a typed metadata value and whether key was present.
*/
func PeekOK[T any](artifact *Artifact, key string) (T, bool) {
	if artifact == nil {
		return *new(T), false
	}

	metadata, err := artifact.Attributes()

	if errnie.Error(err) != nil {
		return *new(T), false
	}

	for idx := range metadata.Len() {
		meta := metadata.At(idx)
		metaKey, keyErr := meta.Key()

		if keyErr != nil {
			continue
		}

		if metaKey == key {
			return Peek[T](artifact, key), true
		}
	}

	return *new(T), false
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

// SetMetaValue sets a metadata value with the appropriate type
func (artifact *Artifact) SetMetaValue(key string, val any) error {
	errnie.Debug("datura.SetMetaValue", "key", key)

	// Create a new option function
	setOption := func(artifact *Artifact) {
		var (
			mdList    Artifact_Attribute_List
			newMdList Artifact_Attribute_List
			err       error
		)

		// Get existing metadata
		if mdList, err = artifact.Attributes(); err != nil {
			errnie.Error(err)
			return
		}

		// Create new metadata list with space for one more item
		if newMdList, err = artifact.NewAttributes(
			int32(mdList.Len() + 1),
		); err != nil {
			errnie.Error(err)
			return
		}

		// Copy existing metadata
		for idx := range mdList.Len() {
			if err = newMdList.Set(idx, mdList.At(idx)); err != nil {
				errnie.Error(err)
				return
			}
		}

		// Add the new item
		item := newMdList.At(newMdList.Len() - 1)
		if err = item.SetKey(key); err != nil {
			errnie.Error(err)
			return
		}

		// Set value based on type
		switch v := val.(type) {
		case string:
			item.Value().SetTextValue(v)
		case int:
			item.Value().SetIntValue(int64(v))
		case int64:
			item.Value().SetIntValue(v)
		case float64:
			item.Value().SetFloatValue(v)
		case bool:
			item.Value().SetBoolValue(v)
		case []byte:
			item.Value().SetBinaryValue(v)
		default:
			// Default to string representation
			item.Value().SetTextValue(fmt.Sprintf("%v", v))
		}
	}

	// Apply the option
	setOption(artifact)
	return nil
}
