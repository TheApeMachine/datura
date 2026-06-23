package datura

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
)

var Empty = Artifact{}

func Acquire(
	origin string,
	artifactType Artifact_Type,
) *Artifact {
	_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))

	if errnie.Error(err) != nil {
		return nil
	}

	artifact := errnie.Does(func() (Artifact, error) {
		return NewRootArtifact(seg)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "artifact acquire failed", err))
	}).Value()

	errnie.Error(artifact.SetUuid([]byte(uuid.NewString())))
	artifact.SetUuid([]byte(uuid.NewString()))
	artifact.SetTimestamp(time.Now().UnixNano())
	artifact.SetAttributes([]byte("{}"))
	artifact.SetTimestamp(time.Now().UnixNano())
	artifact.SetOrigin(origin)
	artifact.SetType(artifactType)

	return &artifact
}

func (artifact *Artifact) Prefix(schemas ...string) []byte {
	var builder strings.Builder

	builder.Grow(256)

	for _, schema := range schemas {
		switch schema {
		case "role":
			if role, err := artifact.Role(); err == nil && role != "" {
				builder.WriteString(role)
				builder.WriteByte('/')
			}
		case "scope":
			if scope, err := artifact.Scope(); err == nil && scope != "" {
				builder.WriteString(scope)
				builder.WriteByte('/')
			}
		case "origin":
			if origin, err := artifact.Origin(); err == nil && origin != "" {
				builder.WriteString(origin)
				builder.WriteByte('/')
			}
		case "destination":
			if destination, err := artifact.Destination(); err == nil && destination != "" {
				builder.WriteString(destination)
				builder.WriteByte('/')
			}
		case "timestamp":
			if timestamp := artifact.Timestamp(); timestamp > 0 {
				observed := time.Unix(0, timestamp).UTC()
				builder.WriteString(observed.Format("2006/01/02"))
				builder.WriteByte('/')
			}
		case "uuid":
			if uuidBytes, err := artifact.Uuid(); err == nil && len(uuidBytes) > 0 {
				builder.Write(uuidBytes)
				builder.WriteByte('/')
			}
		default:
			builder.WriteString(schema)
			builder.WriteByte('/')
		}
	}

	if len(schemas) > 0 {
		out := builder.String()

		if len(out) > 0 && out[len(out)-1] == '/' {
			out = out[:len(out)-1]
		}

		return []byte(out)
	}

	if role, err := artifact.Role(); err == nil && role != "" && !slices.Contains(schemas, "role") {
		builder.WriteString(role)
		builder.WriteByte('/')
	}

	if scope, err := artifact.Scope(); err == nil && scope != "" && !slices.Contains(schemas, "scope") {
		builder.WriteString(scope)
		builder.WriteByte('/')
	}

	if origin, err := artifact.Origin(); err == nil && origin != "" && !slices.Contains(schemas, "origin") {
		builder.WriteString(origin)
		builder.WriteByte('/')
	}

	if destination, err := artifact.Destination(); err == nil && destination != "" && !slices.Contains(schemas, "destination") {
		builder.WriteString(destination)
		builder.WriteByte('/')
	}

	if timestamp := artifact.Timestamp(); timestamp > 0 && !slices.Contains(schemas, "timestamp") {
		observed := time.Unix(0, timestamp).UTC()
		builder.WriteString(observed.Format("2006/01/02"))
		builder.WriteByte('/')
	}

	if uuidBytes, err := artifact.Uuid(); err == nil && len(uuidBytes) > 0 && !slices.Contains(schemas, "uuid") {
		builder.Write(uuidBytes)
	}

	out := builder.String()

	if len(out) > 0 && out[len(out)-1] == '/' {
		out = out[:len(out)-1]
	}

	if !slices.Contains(schemas, "type") {
		out += "."

		artifactType := artifact.Type()

		if artifactType != 0 {
			out += artifactType.String()

			return []byte(out)
		}
	}

	if !slices.Contains(schemas, "uuid") {
		uuidBytes, uuidErr := artifact.Uuid()

		if uuidErr == nil && len(uuidBytes) > 0 {
			out += "json"
		}
	}

	return []byte(out)
}

func (artifact *Artifact) Release() {}

func (artifact *Artifact) Inspect(headers ...string) *Artifact {
	if os.Getenv("DATURA_INSPECT") != "1" {
		return artifact
	}

	origin, _ := artifact.Origin()
	role, _ := artifact.Role()
	scope, _ := artifact.Scope()
	destination, _ := artifact.Destination()
	attributes, _ := artifact.Attributes()
	payload := artifact.DecryptPayload()

	fmt.Println()
	fmt.Println("[" + strings.Join(headers, "/") + "]")
	fmt.Println("prefix      : " + string(artifact.Prefix(headers...)))
	fmt.Println("origin      : " + origin)
	fmt.Println("role        : " + role)
	fmt.Println("scope       : " + scope)
	fmt.Println("destination : " + destination)
	fmt.Println("attributes  : " + string(attributes))
	fmt.Println("payload     : " + string(payload))
	fmt.Println()

	return artifact
}

func (artifact *Artifact) Log() []any {
	origin, _ := artifact.Origin()
	role, _ := artifact.Role()
	scope, _ := artifact.Scope()
	destination, _ := artifact.Destination()

	return []any{
		"origin", origin,
		"role", role,
		"scope", scope,
		"destination", destination,
	}
}

func (artifact *Artifact) WithDestination(destination string) *Artifact {
	errnie.Error(artifact.SetDestination(destination))
	return artifact
}

func (artifact *Artifact) WithPayload(payload []byte) *Artifact {
	if len(payload) == 0 {
		origin, _ := artifact.Origin()
		role, _ := artifact.Role()
		scope, _ := artifact.Scope()
		destination, _ := artifact.Destination()

		errnie.Error(errnie.Err(
			errnie.Validation, "payload is empty", nil,
		).With(
			"origin", origin,
			"role", role,
			"scope", scope,
			"destination", destination,
		))

		return nil
	}

	if artifact.HasEncryptedPayload() {
		artifact.compactPayloadTarget()
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

func (artifact *Artifact) compactPayloadTarget() {
	_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))

	if errnie.Error(err) != nil {
		return
	}

	next, err := NewRootArtifact(seg)

	if errnie.Error(err) != nil {
		return
	}

	next.SetTimestamp(artifact.Timestamp())
	next.SetType(artifact.Type())

	copyData := func(read func() ([]byte, error), write func([]byte) error) {
		value, readErr := read()

		if readErr != nil || len(value) == 0 {
			return
		}

		errnie.Error(write(append([]byte(nil), value...)))
	}

	copyText := func(read func() (string, error), write func(string) error) {
		value, readErr := read()

		if readErr != nil || value == "" {
			return
		}

		errnie.Error(write(value))
	}

	copyData(artifact.Uuid, next.SetUuid)
	copyData(artifact.Checksum, next.SetChecksum)
	copyData(artifact.PseudonymHash, next.SetPseudonymHash)
	copyData(artifact.MerkleRoot, next.SetMerkleRoot)
	copyData(artifact.Attributes, next.SetAttributes)
	copyData(artifact.Signature, next.SetSignature)
	copyText(artifact.Origin, next.SetOrigin)
	copyText(artifact.Destination, next.SetDestination)
	copyText(artifact.Role, next.SetRole)
	copyText(artifact.Scope, next.SetScope)

	*artifact = next
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
WithAttributesAsPayload copies staged attributes into the encrypted payload slot.
*/
func (artifact *Artifact) WithAttributesAsPayload() *Artifact {
	encoded, err := artifact.Attributes()

	if err != nil || len(encoded) == 0 {
		return artifact
	}

	return artifact.WithPayload(encoded)
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
