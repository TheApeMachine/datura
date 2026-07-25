package dmt

import (
	"bytes"
	"encoding/binary"
	"math"
)

const (
	childIndexNamespace       = "idx/c/"
	analogIndexNamespace      = "idx/a/"
	defaultChildIndexCapacity = 32
	analogCandidateCapacity   = 4096
)

/*
readChildProbabilityIndex loads a namespace-scoped child probability index when present.
*/
func (tree *Tree) readChildProbabilityIndex(
	parentPrefix []byte,
	targetBuffer []LookaheadPrediction,
	limit int,
) ([]LookaheadPrediction, bool) {
	if tree == nil || len(parentPrefix) == 0 {
		return targetBuffer, false
	}

	value, found := tree.getRaw(childIndexStorageKey(parentPrefix))

	if !found {
		return targetBuffer, false
	}

	predictions, _, ok := unmarshalChildIndex(value)

	if !ok {
		return targetBuffer, false
	}

	if limit > 0 && len(predictions) > limit {
		predictions = predictions[:limit]
	}

	targetBuffer = targetBuffer[:0]

	for _, prediction := range predictions {
		targetBuffer = append(targetBuffer, LookaheadPrediction{
			Token:       append([]byte(nil), prediction.Token...),
			Probability: prediction.Probability,
		})
	}

	return targetBuffer, true
}

/*
storeChildProbabilityIndex persists top-k child probabilities for one parent prefix.
Indexes are derived, ephemeral radix entries and are not written to the WAL.
*/
func (tree *Tree) storeChildProbabilityIndex(
	parentPrefix []byte,
	predictions []LookaheadPrediction,
	truncated bool,
) {
	if tree == nil || len(parentPrefix) == 0 || len(predictions) == 0 {
		return
	}

	tree.insertEphemeral(childIndexStorageKey(parentPrefix), marshalChildIndex(predictions, truncated))
}

/*
refreshChildProbabilityIndex rebuilds the child probability index for parentPrefix.
*/
func (tree *Tree) refreshChildProbabilityIndex(childKey []byte) {
	if tree == nil {
		return
	}

	storagePrefix := parentStoragePrefix(childKey)

	if len(storagePrefix) == 0 {
		return
	}

	buffer := make([]LookaheadPrediction, 0, defaultChildIndexCapacity)
	root := tree.loadRoot()
	iterator := root.Root().Iterator()

	iterator.SeekPrefix(storagePrefix)

	truncated := false

	for key, value, ok := iterator.Next(); ok; key, value, ok = iterator.Next() {
		if !bytes.HasPrefix(key, storagePrefix) {
			break
		}

		tokenSuffix, isChild := immediateTokenSuffix(storagePrefix, key)

		if !isChild {
			continue
		}

		weight := UnmarshalWeight(value)
		buffer, truncated = insertTopKPrediction(
			buffer,
			tokenSuffix,
			weight.Probability,
			defaultChildIndexCapacity,
		)
	}

	if len(buffer) == 0 {
		tree.deleteEphemeral(childIndexStorageKey(storagePrefix))

		return
	}

	tree.storeChildProbabilityIndex(storagePrefix, buffer, truncated)
}

func parentStoragePrefix(childKey []byte) []byte {
	if bytes.HasPrefix(childKey, []byte(sensoryNamespace)) {
		sequence, mapped := sequenceFromSensoryKey(childKey)

		if !mapped {
			return nil
		}

		return sensoryStorageKey(parentContextPath(sequence))
	}

	parentPrefix := parentContextPath(childKey)

	if len(parentPrefix) > 0 {
		return parentPrefix
	}

	tokenBoundary := bytes.IndexByte(childKey, '_')

	if tokenBoundary <= 0 {
		return childKey[:0]
	}

	return childKey[:tokenBoundary]
}

/*
registerAnalogCandidate indexes one storage key under each token-boundary prefix.
*/
func (tree *Tree) registerAnalogCandidate(storageKey []byte) {
	if tree == nil || len(storageKey) == 0 {
		return
	}

	candidateKey := append([]byte(nil), storageKey...)
	tokenStart := 0

	for index := 0; index <= len(storageKey); index++ {
		if index < len(storageKey) && storageKey[index] != '_' {
			continue
		}

		if index == tokenStart {
			tokenStart = index + 1

			continue
		}

		tokenPrefix := storageKey[:index]
		indexKey := analogIndexStorageKey(tokenPrefix)
		existing, found := tree.Get(indexKey)
		truncated := false
		candidates := [][]byte{}

		if found {
			candidates, truncated = unmarshalAnalogIndex(existing)
		}

		candidates, truncated = appendAnalogCandidate(candidates, candidateKey, truncated)
		tree.insertEphemeral(indexKey, marshalAnalogIndex(candidates, truncated))
		tokenStart = index + 1
	}
}

/*
collectAnalogCandidates returns indexed candidates for unknownKey in longest-prefix order.
*/
func (tree *Tree) collectAnalogCandidates(unknownKey []byte) ([][]byte, bool) {
	if tree == nil || len(unknownKey) == 0 {
		return nil, false
	}

	namespace := namespacePrefixOf(unknownKey)
	merged := make([][]byte, 0)
	truncated := false
	tokenStart := 0

	for index := 0; index <= len(unknownKey); index++ {
		if index < len(unknownKey) && unknownKey[index] != '_' {
			continue
		}

		if index == tokenStart {
			tokenStart = index + 1

			continue
		}

		tokenPrefix := unknownKey[:index]
		value, found := tree.getRaw(analogIndexStorageKey(tokenPrefix))

		if !found {
			tokenStart = index + 1

			continue
		}

		candidates, candidateTruncated := unmarshalAnalogIndex(value)
		truncated = truncated || candidateTruncated

		for _, candidate := range candidates {
			if len(namespace) > 0 && !bytes.HasPrefix(candidate, namespace) {
				continue
			}

			merged = appendUniqueCandidate(merged, candidate)
		}

		tokenStart = index + 1
	}

	return merged, truncated
}

func appendAnalogCandidate(candidates [][]byte, candidate []byte, truncated bool) ([][]byte, bool) {
	for _, existing := range candidates {
		if bytes.Equal(existing, candidate) {
			return candidates, truncated
		}
	}

	if len(candidates) >= analogCandidateCapacity {
		return candidates, true
	}

	return append(candidates, append([]byte(nil), candidate...)), truncated
}

func appendUniqueCandidate(candidates [][]byte, candidate []byte) [][]byte {
	for _, existing := range candidates {
		if bytes.Equal(existing, candidate) {
			return candidates
		}
	}

	return append(candidates, candidate)
}

func marshalAnalogIndex(candidates [][]byte, truncated bool) []byte {
	header := uint32(len(candidates))

	if truncated {
		header |= 1 << 31
	}

	size := 4

	for _, candidate := range candidates {
		size += 4 + len(candidate)
	}

	buffer := make([]byte, size)
	binary.LittleEndian.PutUint32(buffer[0:4], header)
	offset := 4

	for _, candidate := range candidates {
		binary.LittleEndian.PutUint32(buffer[offset:offset+4], uint32(len(candidate)))
		offset += 4
		offset += copy(buffer[offset:], candidate)
	}

	return buffer
}

func unmarshalAnalogIndex(buffer []byte) ([][]byte, bool) {
	if len(buffer) < 4 {
		return nil, false
	}

	rawHeader := binary.LittleEndian.Uint32(buffer[0:4])
	truncated := rawHeader&(1<<31) != 0
	count := rawHeader &^ (1 << 31)
	candidates := make([][]byte, 0, count)
	offset := 4

	for index := uint32(0); index < count; index++ {
		if offset+4 > len(buffer) {
			return candidates, truncated
		}

		candidateLength := binary.LittleEndian.Uint32(buffer[offset : offset+4])
		offset += 4

		if offset+int(candidateLength) > len(buffer) {
			return candidates, truncated
		}

		candidates = append(
			candidates,
			append([]byte(nil), buffer[offset:offset+int(candidateLength)]...),
		)
		offset += int(candidateLength)
	}

	return candidates, truncated
}

func unmarshalChildIndex(buffer []byte) ([]LookaheadPrediction, bool, bool) {
	if len(buffer) < 4 {
		return nil, false, false
	}

	rawCount := binary.LittleEndian.Uint32(buffer[0:4])
	truncated := rawCount&(1<<31) != 0
	count := rawCount &^ (1 << 31)

	if count == 0 {
		return nil, truncated, true
	}

	predictions := make([]LookaheadPrediction, 0, count)
	offset := 4

	for index := uint32(0); index < count; index++ {
		if offset+4 > len(buffer) {
			return nil, truncated, false
		}

		tokenLength := binary.LittleEndian.Uint32(buffer[offset : offset+4])
		offset += 4

		if offset+int(tokenLength)+8 > len(buffer) {
			return nil, truncated, false
		}

		token := append([]byte(nil), buffer[offset:offset+int(tokenLength)]...)
		offset += int(tokenLength)
		probability := math.Float64frombits(binary.LittleEndian.Uint64(buffer[offset : offset+8]))
		offset += 8

		predictions = append(predictions, LookaheadPrediction{
			Token:       token,
			Probability: probability,
		})
	}

	return predictions, truncated, true
}

func marshalChildIndex(predictions []LookaheadPrediction, truncated bool) []byte {
	header := uint32(len(predictions))

	if truncated {
		header |= 1 << 31
	}

	size := 4

	for _, prediction := range predictions {
		size += 4 + len(prediction.Token) + 8
	}

	buffer := make([]byte, size)
	binary.LittleEndian.PutUint32(buffer[0:4], header)
	offset := 4

	for _, prediction := range predictions {
		binary.LittleEndian.PutUint32(buffer[offset:offset+4], uint32(len(prediction.Token)))
		offset += 4
		offset += copy(buffer[offset:], prediction.Token)
		binary.LittleEndian.PutUint64(
			buffer[offset:offset+8],
			math.Float64bits(prediction.Probability),
		)
		offset += 8
	}

	return buffer
}

func childIndexStorageKey(parentPrefix []byte) []byte {
	storageKey := make([]byte, len(childIndexNamespace)+len(parentPrefix))
	copy(storageKey, childIndexNamespace)
	copy(storageKey[len(childIndexNamespace):], parentPrefix)

	return storageKey
}

func analogIndexStorageKey(tokenPrefix []byte) []byte {
	storageKey := make([]byte, len(analogIndexNamespace)+len(tokenPrefix))
	copy(storageKey, analogIndexNamespace)
	copy(storageKey[len(analogIndexNamespace):], tokenPrefix)

	return storageKey
}

/*
insertTopKPrediction keeps the highest-probability child branches up to limit.
*/
func insertTopKPrediction(
	predictions []LookaheadPrediction,
	token []byte,
	probability float64,
	limit int,
) ([]LookaheadPrediction, bool) {
	if limit <= 0 {
		limit = defaultChildIndexCapacity
	}

	existingIndex := predictionIndexForToken(predictions, token)

	if existingIndex >= 0 {
		if probability > predictions[existingIndex].Probability {
			predictions[existingIndex].Probability = probability
			predictions = sortPredictionsDescending(predictions)
		}

		return predictions, len(predictions) >= limit
	}

	candidate := LookaheadPrediction{
		Token:       append([]byte(nil), token...),
		Probability: probability,
	}

	if len(predictions) < limit {
		predictions = append(predictions, candidate)
		predictions = sortPredictionsDescending(predictions)

		return predictions, false
	}

	if probability <= predictions[len(predictions)-1].Probability {
		return predictions, true
	}

	predictions[len(predictions)-1] = candidate
	predictions = sortPredictionsDescending(predictions)

	return predictions, true
}

func sortPredictionsDescending(predictions []LookaheadPrediction) []LookaheadPrediction {
	for left := 1; left < len(predictions); left++ {
		current := predictions[left]
		right := left

		for right > 0 && current.Probability > predictions[right-1].Probability {
			predictions[right] = predictions[right-1]
			right--
		}

		predictions[right] = current
	}

	return predictions
}

func (tree *Tree) maintainCognitiveIndexes(key []byte, deleted bool) {
	if tree == nil || len(key) == 0 {
		return
	}

	if bytes.HasPrefix(key, []byte(childIndexNamespace)) ||
		bytes.HasPrefix(key, []byte(analogIndexNamespace)) {
		return
	}

	tree.refreshChildProbabilityIndex(key)

	if deleted {
		return
	}

	tree.registerAnalogCandidate(key)
}

/*
insertEphemeral publishes derived index values without durable WAL writes.
*/
func (tree *Tree) insertEphemeral(key, value []byte) {
	if tree == nil {
		return
	}

	for {
		oldRoot := tree.loadRoot()
		newRoot, _, _ := oldRoot.Insert(key, value)

		if tree.root.CompareAndSwap(oldRoot, newRoot) {
			return
		}
	}
}

/*
deleteEphemeral removes derived index values without durable WAL writes.
*/
func (tree *Tree) deleteEphemeral(key []byte) {
	if tree == nil {
		return
	}

	for {
		oldRoot := tree.loadRoot()
		newRoot, _, ok := oldRoot.Delete(key)

		if !ok {
			return
		}

		if tree.root.CompareAndSwap(oldRoot, newRoot) {
			return
		}
	}
}
