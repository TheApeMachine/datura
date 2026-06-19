package datura

import (
	"errors"
	"strings"
	"sync"
	"time"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/bytedance/sonic"
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

		artifact.SetUuid([]byte(uuid.NewString()))
		artifact.SetTimestamp(time.Now().UnixNano())

		return &artifact
	},
}

var Empty = Artifact{}

func Acquire(
	origin string,
	artifactType Artifact_Type,
) *Artifact {
	artifact := artifactPool.Get().(*Artifact)

	artifact.SetOrigin(origin)
	artifact.SetType(artifactType)

	return artifact
}

func (artifact *Artifact) Prefix(schemas ...string) []byte {
	var builder strings.Builder

	builder.Grow(256)

	for _, schema := range schemas {
		switch schema {
		case "timestamp":
			if timestamp := artifact.Timestamp(); timestamp > 0 {
				observed := time.Unix(0, timestamp).UTC()
				builder.WriteString(observed.Format("2006/01/02"))
				builder.WriteByte('/')
			}
		}
	}

	if role, err := artifact.Role(); err == nil && role != "" {
		builder.WriteString(role)
		builder.WriteByte('/')
	}

	if scope, err := artifact.Scope(); err == nil && scope != "" {
		builder.WriteString(scope)
		builder.WriteByte('/')
	}

	if origin, err := artifact.Origin(); err == nil && origin != "" {
		builder.WriteString(origin)
		builder.WriteByte('/')
	}

	if destination, err := artifact.Destination(); err == nil && destination != "" {
		builder.WriteString(destination)
		builder.WriteByte('/')
	}

	if timestamp := artifact.Timestamp(); timestamp > 0 {
		observed := time.Unix(0, timestamp).UTC()
		builder.WriteString(observed.Format("2006/01/02"))
		builder.WriteByte('/')
	}

	if uuidBytes, err := artifact.Uuid(); err == nil && len(uuidBytes) > 0 {
		builder.Write(uuidBytes)
	}

	out := builder.String()

	if len(out) > 0 && out[len(out)-1] == '/' {
		out = out[:len(out)-1]
	}

	out += "."

	artifactType := artifact.Type()

	if artifactType != 0 {
		out += artifactType.String()

		return []byte(out)
	}

	uuidBytes, uuidErr := artifact.Uuid()

	if uuidErr == nil && len(uuidBytes) > 0 {
		out += "json"
	}

	return []byte(out)
}

func (artifact *Artifact) Release() {
	if artifact == nil {
		return
	}

	artifactPool.Put(artifact)
}

func (artifact *Artifact) WithDestination(destination string) *Artifact {
	errnie.Error(artifact.SetDestination(destination))
	return artifact
}

func (artifact *Artifact) WithPayload(payload []byte) *Artifact {
	if len(payload) == 0 {
		errnie.Error(errors.New("payload is empty"))
		return nil
	}

	cryptoSuite := NewCryptoSuite()
	cipherLen := cryptoSuite.EncryptedPayloadSize(len(payload))

	if errnie.Error(artifact.SetEncryptedPayload(make([]byte, cipherLen))) != nil {
		return nil
	}

	if errnie.Error(artifact.SetEncryptedKey(make([]byte, aesKeyBytes))) != nil {
		return nil
	}

	if errnie.Error(artifact.SetEphemeralPublicKey(make([]byte, p256PubKeyBytes))) != nil {
		return nil
	}

	encPayloadBuf, err := artifact.EncryptedPayload()

	if errnie.Error(err) != nil {
		return nil
	}

	encKeyBuf, err := artifact.EncryptedKey()

	if errnie.Error(err) != nil {
		return nil
	}

	ephemeralKeyBuf, err := artifact.EphemeralPublicKey()

	if errnie.Error(err) != nil {
		return nil
	}

	if errnie.Error(cryptoSuite.EncryptPayloadDirect(
		encPayloadBuf,
		encKeyBuf,
		ephemeralKeyBuf,
		payload,
	)) != nil {
		return nil
	}

	return artifact
}

func (artifact *Artifact) WithAttributes(attributes Map[any]) *Artifact {
	encoded := errnie.Does(func() ([]byte, error) {
		return sonic.Marshal(attributes)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "attributes marshal failed", err))
	}).Value()

	if len(encoded) == 0 {
		return artifact
	}

	errnie.Error(artifact.SetAttributes(encoded))

	return artifact
}

func (artifact *Artifact) WithSignature(signature []byte) *Artifact {
	errnie.Error(artifact.SetSignature(signature))
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

/*
WithAttribute stores one attribute value using dotted key paths.
*/
func (artifact *Artifact) WithAttribute(key string, value any) *Artifact {
	if key == "" {
		return artifact
	}

	if strings.Contains(key, ".") {
		segments := strings.Split(key, ".")
		path := make([]any, len(segments))

		for index, segment := range segments {
			path[index] = segment
		}

		artifact.Poke(value, path...)

		return artifact
	}

	artifact.Poke(value, key)

	return artifact
}

/*
Marshal serializes the artifact capnp wire frame.
*/
func (artifact *Artifact) Marshal() []byte {
	wire, err := artifact.Message().Marshal()

	if err != nil {
		return nil
	}

	return wire
}

func (artifact *Artifact) WithError(err error) *Artifact {
	artifact.WithPayload([]byte(err.Error()))
	return artifact
}
