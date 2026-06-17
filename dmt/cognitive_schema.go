package dmt

import "encoding/binary"

const (
	sensoryNamespace     = "s/"
	episodicNamespace    = "e/"
	basinNamespace       = "b/"
	episodicTimestampLen = 8
	episodicHeaderLen    = len(episodicNamespace) + episodicTimestampLen
)

/*
CognitiveState is the packed count/probability layout stored in radix values.
*/
type CognitiveState = PackedWeight

/*
MarshalCognitive encodes a cognitive state into a fixed 16-byte buffer.
*/
func MarshalCognitive(state CognitiveState) []byte {
	return MarshalWeight(state.Count, state.Probability)
}

/*
UnmarshalCognitive decodes a cognitive state buffer.
*/
func UnmarshalCognitive(buffer []byte) CognitiveState {
	return UnmarshalWeight(buffer)
}

func sensoryStorageKey(sequence []byte) []byte {
	storageKey := make([]byte, len(sensoryNamespace)+len(sequence))
	copy(storageKey, sensoryNamespace)
	copy(storageKey[len(sensoryNamespace):], sequence)

	return storageKey
}

func episodicStorageKey(timestamp uint64, sequence []byte) []byte {
	storageKey := make([]byte, episodicHeaderLen+len(sequence))
	copy(storageKey, episodicNamespace)
	binary.BigEndian.PutUint64(storageKey[len(episodicNamespace):episodicHeaderLen], timestamp)
	copy(storageKey[episodicHeaderLen:], sequence)

	return storageKey
}

func basinStorageKey(class []byte, sequence []byte) []byte {
	storageKey := make([]byte, len(basinNamespace)+len(class)+1+len(sequence))
	offset := copy(storageKey, basinNamespace)
	offset += copy(storageKey[offset:], class)
	storageKey[offset] = '/'
	copy(storageKey[offset+1:], sequence)

	return storageKey
}

func sequenceFromSensoryKey(storageKey []byte) ([]byte, bool) {
	if len(storageKey) <= len(sensoryNamespace) {
		return nil, false
	}

	if string(storageKey[:len(sensoryNamespace)]) != sensoryNamespace {
		return nil, false
	}

	return storageKey[len(sensoryNamespace):], true
}

func timestampFromEpisodicKey(storageKey []byte) (uint64, []byte, bool) {
	if len(storageKey) < episodicHeaderLen {
		return 0, nil, false
	}

	if string(storageKey[:len(episodicNamespace)]) != episodicNamespace {
		return 0, nil, false
	}

	timestamp := binary.BigEndian.Uint64(storageKey[len(episodicNamespace):episodicHeaderLen])
	sequence := storageKey[episodicHeaderLen:]

	return timestamp, sequence, true
}
