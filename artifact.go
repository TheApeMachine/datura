package datura

import (
	"errors"
	"fmt"
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

		return &pooledArtifact{
			Artifact: artifact,
		}
	},
}

var artifactPoolIndex sync.Map

/*
pooledArtifact groups a Cap'n Proto artifact with pool bookkeeping.
The exported surface remains *Artifact pointers into the embedded struct.
*/
type pooledArtifact struct {
	Artifact
}

func (pooled *pooledArtifact) resetForPool() {
	resetArtifactStreamState(&pooled.Artifact)

	segment, err := pooled.Artifact.Message().Reset(capnp.SingleSegment(nil))

	if errnie.Error(err) != nil {
		return
	}

	fresh, err := NewRootArtifact(segment)

	if errnie.Error(err) != nil {
		return
	}

	pooled.Artifact = fresh
}

func Acquire(
	origin string,
	artifactType Artifact_Type,
) *Artifact {
	pooled := artifactPool.Get()

	if pooled == nil {
		return nil
	}

	pa, ok := pooled.(*pooledArtifact)

	if !ok {
		return nil
	}

	if errnie.Error(pa.SetUuid([]byte(uuid.NewString()))) != nil {
		return nil
	}

	pa.SetTimestamp(time.Now().UnixNano())

	if errnie.Error(pa.SetOrigin(origin)) != nil {
		return nil
	}

	pa.SetType(artifactType)
	artifactPoolIndex.Store(&pa.Artifact, pa)

	return &pa.Artifact
}

func (artifact *Artifact) Prefix() string {
	var builder strings.Builder

	builder.Grow(256)

	var numBuf [32]byte

	if origin, err := artifact.Origin(); err == nil && origin != "" {
		builder.WriteString(origin)
		builder.WriteByte('/')
	}

	if destination, err := artifact.Destination(); err == nil && destination != "" {
		builder.WriteString(destination)
		builder.WriteByte('/')
	}

	if role, err := artifact.Role(); err == nil && role != "" {
		builder.WriteString(role)
		builder.WriteByte('/')
	}

	if scope, err := artifact.Scope(); err == nil && scope != "" {
		builder.WriteString(scope)
		builder.WriteByte('/')
	}

	if timestamp := artifact.Timestamp(); timestamp > 0 {
		base36 := strconv.AppendInt(numBuf[:0], timestamp, 36)
		builder.Write(base36)
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

	if artifactType := artifact.Type(); artifactType != 0 {
		out += artifactType.String()
	}

	return out
}

func (artifact *Artifact) Release() {
	if artifact == nil {
		return
	}

	pooled, ok := artifactPoolIndex.LoadAndDelete(artifact)

	if !ok {
		resetArtifactStreamState(artifact)
		return
	}

	pa := pooled.(*pooledArtifact)
	pa.resetForPool()
	artifactPool.Put(pa)
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

func (artifact *Artifact) WithAttrubutes(attributes map[string]any) *Artifact {
	var (
		mdList    Artifact_Attribute_List
		newMdList Artifact_Attribute_List
		err       error
	)

	if mdList, err = artifact.Attributes(); errnie.Error(err) != nil {
		return nil
	}

	if newMdList, err = (*artifact).NewAttributes(
		int32(mdList.Len() + len(attributes)),
	); errnie.Error(err) != nil {
		return nil
	}

	for idx := range mdList.Len() {
		if errnie.Error(newMdList.Set(idx, mdList.At(idx))) != nil {
			return nil
		}
	}

	nextIdx := mdList.Len()

	for key, value := range attributes {
		item := newMdList.At(nextIdx)
		nextIdx++

		if errnie.Error(item.SetKey(key)) != nil {
			return nil
		}

		switch v := value.(type) {
		case string:
			if errnie.Error(item.Value().SetTextValue(v)) != nil {
				return nil
			}
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
			item.Value().SetTextValue(fmt.Sprintf("%v", v))
		}

		syncArtifactCacheEntry(artifact, key, value)
	}

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

func (artifact *Artifact) Poke(key string, value string) *Artifact {
	errnie.Error(artifact.SetMetaValue(key, value))
	return artifact
}

func (artifact *Artifact) Metadata() (Artifact_Attribute_List, error) {
	return artifact.Attributes()
}

func (artifact *Artifact) WithAttribute(key string, value any) *Artifact {
	errnie.Error(artifact.SetMetaValue(key, value))
	return artifact
}

func (artifact *Artifact) WithError(err error) *Artifact {
	artifact.WithPayload([]byte(err.Error()))
	return artifact
}
