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

		artifact.SetUuid([]byte(uuid.NewString()))
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
		prefix = append(prefix, string(uuid))
	}

	out := strings.Join(prefix, "/") + "."

	if t := artifact.Type(); t != 0 {
		out += t.String()
	}

	return out
}

func (artifact *Artifact) Release() {
	if artifact == nil {
		return
	}
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

	encryptedPayload, encryptedKey, ephemeralPubKey, err := cryptoSuite.EncryptPayload(payload)

	if errnie.Error(err) != nil {
		return nil
	}

	if errnie.Error(artifact.SetEncryptedPayload(encryptedPayload)) != nil {
		return nil
	}

	if errnie.Error(artifact.SetEncryptedKey(encryptedKey)) != nil {
		return nil
	}

	if errnie.Error(artifact.SetEphemeralPublicKey(ephemeralPubKey)) != nil {
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
	errnie.Debug("datura.WithMeta")
	errnie.Error(artifact.SetMetaValue(key, value))
	return artifact
}

func (artifact *Artifact) WithError(err error) *Artifact {
	errnie.Debug("datura.WithError")
	artifact.WithPayload([]byte(err.Error()))
	return artifact
}
