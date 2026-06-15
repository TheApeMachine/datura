package datura

import (
	"strconv"
	"strings"
	"sync"
	"time"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
)

var artifactPool = sync.Pool{
	New: func() any {
		arena := capnp.SingleSegment(nil)

		_, seg, err := capnp.NewMessage(arena)

		if errnie.Error(err) != nil {
			return nil
		}

		artifact, err := NewRootArtifact(seg)

		if errnie.Error(err) != nil {
			return nil
		}

		artifact.SetUuid(uuid.NewString())
		artifact.SetTimestamp(time.Now().UnixNano())

		stored := artifact

		return &stored
	},
}

func Acquire(
	origin string,
	artifactType Artifact_Type,
) *Artifact {
	pooled := artifactPool.Get()
	if pooled == nil {
		return nil
	}

	artifact, ok := pooled.(*Artifact)
	if !ok {
		return nil
	}

	if errnie.Error(artifact.SetOrigin(origin)) != nil {
		return nil
	}

	artifact.SetType(artifactType)

	return artifact
}

func (artifact *Artifact) Prefix() string {
	prefix := make([]string, 0)

	if origin, err := artifact.Origin(); err == nil {
		prefix = append(prefix, origin)
	}

	if destination, err := artifact.Destination(); err == nil {
		prefix = append(prefix, destination)
	}

	if role, err := artifact.Role(); err == nil {
		prefix = append(prefix, role)
	}

	if scope, err := artifact.Scope(); err == nil {
		prefix = append(prefix, scope)
	}

	if ts := artifact.Timestamp(); ts > 0 {
		prefix = append(prefix, strconv.FormatInt(ts, 36))
	}

	if uuid, err := artifact.Uuid(); err == nil {
		prefix = append(prefix, uuid)
	}

	out := strings.Join(prefix, "/") + "."

	if t := artifact.Type(); t != 0 {
		out += t.String()
	}

	return out
}

func (artifact *Artifact) WithError(errnieError *errnie.ErrnieError) *Artifact {
	artifactErr, err := NewArtifact_Error(artifact.Segment())

	if errnie.Error(err) != nil {
		return artifact
	}

	switch errnieError.Kind {
	case errnie.Unknown:
		artifactErr.SetType(Artifact_Error_Type_unknown)
	case errnie.Validation:
		artifactErr.SetType(Artifact_Error_Type_validation)
	}

	artifactErr.SetTimestamp(errnieError.Timestamp)
	errnie.Error(artifactErr.SetMessage_(errnieError.Message))
	errnie.Error(artifact.SetError(artifactErr))

	return artifact
}

func (artifact *Artifact) WithDestination(destination string) *Artifact {
	errnie.Error(artifact.SetDestination(destination))
	return artifact
}

func (artifact *Artifact) WithRole(role string) *Artifact {
	errnie.Error(artifact.SetRole(role))
	return artifact
}

func (artifact *Artifact) WithScope(scope string) *Artifact {
	errnie.Error(artifact.SetScope(scope))
	return artifact
}

func (artifact *Artifact) WithPayload(data []byte) *Artifact {
	errnie.Error(artifact.SetPayload(data))
	return artifact
}

func (artifact *Artifact) Release() {
	if artifact == nil {
		return
	}
}

/*
Peek retrieves a value from the artifact, starting by looking for an existing field,
and falling back to searching the attribute list.
*/
// Peek returns (value, exists)
func (artifact *Artifact) Peek(key string) string {
	var (
		value string
		data  []byte
		err   error
	)

	// Check if the key corresponds to a top-level field.
	switch key {
	case "id":
		value, err = artifact.Uuid()
	case "type":
		value = artifact.Type().String()
	case "origin":
		value, err = artifact.Origin()
	case "role":
		value, err = artifact.Role()
	case "scope":
		value, err = artifact.Scope()
	case "payload":
		data, err = artifact.Payload()
		value = string(data)
	default:
		// If the key is not a top-level field, look in the attributes list.
		value, err = artifact.getAttributeValue(key)
	}

	if err != nil {
		return errnie.Err(
			errnie.Unknown,
			"artifact peek",
			err,
		).Kind.Error()
	}

	return value
}

// Poke sets a value and returns a boolean indicating success
func (artifact *Artifact) Poke(key, value string) *Artifact {
	var err error

	switch key {
	case "uuid":
		err = artifact.SetUuid(value)
	case "type":
		artifact.SetType(Artifact_TypeFromString(value))
	case "origin":
		err = artifact.SetOrigin(value)
	case "role":
		err = artifact.SetRole(value)
	case "scope":
		err = artifact.SetScope(value)
	case "payload":
		err = artifact.SetPayload([]byte(value))
	default:
		// Check if the attribute already exists and update it, or add a new one
		err = artifact.updateOrAddAttribute(key, value)
	}

	if err != nil {
		errnie.Error(err)
	}

	return artifact
}

// getAttributeValue searches the attribute list for the given key.
func (artifact *Artifact) getAttributeValue(key string) (string, error) {
	attrs, err := artifact.Attributes()

	if errnie.Error(err) != nil {
		return "", err
	}

	// Iterate through the attributes list to find a matching key.
	for i := 0; i < attrs.Len(); i++ {
		attr := attrs.At(i) // Only one return value now.
		attrKey, err := attr.Key()

		if errnie.Error(err) != nil {
			return "", err
		}

		if attrKey == key {
			return attr.Value()
		}
	}

	return "", nil
}

/*
addAttribute adds a new attribute to the artifact.
*/
func (artifact *Artifact) addAttribute(key, value string) error {
	// Retrieve the existing attributes.
	attrs, err := artifact.Attributes()

	if err != nil {
		return errnie.Error(err)
	}

	// Create a new list of attributes, with length
	// increased by 1 to accommodate the new attribute.
	newAttrs, err := NewArtifact_Attribute_List(
		artifact.Segment(), int32(attrs.Len()+1),
	)

	if err != nil {
		return errnie.Error(err)
	}

	// Copy existing attributes to the new list.
	for i := 0; i < attrs.Len(); i++ {
		if err := newAttrs.Set(i, attrs.At(i)); err != nil {
			return errnie.Error(err)
		}
	}

	// Add the new attribute at the last position.
	newAttr := newAttrs.At(attrs.Len())

	if err := newAttr.SetKey(key); err != nil {
		return errnie.Error(err)
	}

	if err := newAttr.SetValue(value); err != nil {
		return errnie.Error(err)
	}

	// Set the updated list of attributes back to the artifact.
	if err := artifact.SetAttributes(newAttrs); err != nil {
		return errnie.Error(err)
	}

	return nil
}

// updateOrAddAttribute updates an existing attribute or adds a new one if it doesn't exist
func (artifact *Artifact) updateOrAddAttribute(key, value string) error {
	attrs, err := artifact.Attributes()
	if err != nil {
		return errnie.Error(err)
	}

	// Check if the attribute already exists
	for i := 0; i < attrs.Len(); i++ {
		attr := attrs.At(i)
		attrKey, err := attr.Key()
		if err != nil {
			return errnie.Error(err)
		}

		if attrKey == key {
			// Update existing attribute
			return attr.SetValue(value)
		}
	}

	// If the attribute doesn't exist, add a new one
	return artifact.addAttribute(key, value)
}
