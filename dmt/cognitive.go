package dmt

import (
	"bytes"
	"encoding/binary"
	"math"
)

const packedWeightSize = 16

/*
PackedWeight holds statistical training telemetry stored as radix tree values.
*/
type PackedWeight struct {
	Count       uint64
	Probability float64
}

/*
MarshalWeight encodes count and probability into a fixed 16-byte layout.
*/
func MarshalWeight(count uint64, probability float64) []byte {
	buffer := make([]byte, packedWeightSize)

	binary.LittleEndian.PutUint64(buffer[0:8], count)
	binary.LittleEndian.PutUint64(buffer[8:16], math.Float64bits(probability))

	return buffer
}

/*
UnmarshalWeight decodes a packed weight buffer.
*/
func UnmarshalWeight(buffer []byte) PackedWeight {
	if len(buffer) < packedWeightSize {
		return PackedWeight{}
	}

	return PackedWeight{
		Count:       binary.LittleEndian.Uint64(buffer[0:8]),
		Probability: math.Float64frombits(binary.LittleEndian.Uint64(buffer[8:16])),
	}
}

/*
SurprisalItem records one token segment and its information-theoretic surprisal.
*/
type SurprisalItem struct {
	Token     []byte
	Surprisal float64
}

/*
LookaheadPrediction records one stochastic branch from an active prefix.
*/
type LookaheadPrediction struct {
	Token       []byte
	Probability float64
}

/*
GetContextWeight fetches the statistical weight stored at contextPath.
*/
func (tree *Tree) GetContextWeight(contextPath []byte) PackedWeight {
	value, found := tree.Get(contextPath)

	if !found {
		return PackedWeight{}
	}

	return UnmarshalWeight(value)
}

/*
InsertContextWeight stores a packed weight at contextPath.
*/
func (tree *Tree) InsertContextWeight(contextPath []byte, weight PackedWeight) (*Tree, bool, error) {
	return tree.Insert(contextPath, MarshalWeight(weight.Count, weight.Probability))
}

/*
GetSurprisal scans an underscore-delimited sequence and scores each token boundary.
*/
func (tree *Tree) GetSurprisal(sequence []byte) []SurprisalItem {
	tokenCount := countTokenBoundaries(sequence)
	items := make([]SurprisalItem, 0, tokenCount)

	tokenStart := 0

	for index := 0; index <= len(sequence); index++ {
		if index < len(sequence) && sequence[index] != '_' {
			continue
		}

		token := sequence[tokenStart:index]

		if len(token) == 0 {
			tokenStart = index + 1

			continue
		}

		currentPath := sequence[:index]
		weight := tree.GetContextWeight(currentPath)
		parentPath := parentContextPath(currentPath)

		items = append(items, SurprisalItem{
			Token:     append([]byte(nil), token...),
			Surprisal: tree.surprisalForWeight(weight, parentPath),
		})

		tokenStart = index + 1
	}

	return items
}

/*
PredictNextTokens performs prefix lookahead into immediate child token branches.
The caller supplies targetBuffer for reuse; capacity bounds result size.
*/
func (tree *Tree) PredictNextTokens(
	prefix []byte,
	targetBuffer []LookaheadPrediction,
) []LookaheadPrediction {
	targetBuffer = targetBuffer[:0]
	root := tree.loadRoot()
	iterator := root.Root().Iterator()

	iterator.SeekPrefix(prefix)

	for key, value, ok := iterator.Next(); ok; key, value, ok = iterator.Next() {
		if !bytes.HasPrefix(key, prefix) {
			break
		}

		tokenSuffix, isChild := immediateTokenSuffix(prefix, key)

		if !isChild {
			continue
		}

		weight := UnmarshalWeight(value)

		if existingIndex := predictionIndexForToken(targetBuffer, tokenSuffix); existingIndex >= 0 {
			if weight.Probability > targetBuffer[existingIndex].Probability {
				targetBuffer[existingIndex].Probability = weight.Probability
			}

			continue
		}

		targetBuffer = append(targetBuffer, LookaheadPrediction{
			Token:       append([]byte(nil), tokenSuffix...),
			Probability: weight.Probability,
		})

		if len(targetBuffer) == cap(targetBuffer) {
			break
		}
	}

	return targetBuffer
}

func (tree *Tree) surprisalForWeight(weight PackedWeight, parentPath []byte) float64 {
	if weight.Probability > 0 {
		return -math.Log2(weight.Probability)
	}

	parentWeight := tree.GetContextWeight(parentPath)
	denominator := float64(parentWeight.Count) + 1.0

	if denominator <= 0 {
		denominator = 1.0
	}

	return -math.Log2(1.0 / denominator)
}

func countTokenBoundaries(sequence []byte) int {
	if len(sequence) == 0 {
		return 0
	}

	boundaryCount := 1

	for _, character := range sequence {
		if character == '_' {
			boundaryCount++
		}
	}

	return boundaryCount
}

func parentContextPath(currentPath []byte) []byte {
	if len(currentPath) == 0 {
		return nil
	}

	parentEnd := bytes.LastIndexByte(currentPath, '_')

	if parentEnd < 0 {
		return nil
	}

	return currentPath[:parentEnd]
}

func immediateTokenSuffix(prefix, key []byte) ([]byte, bool) {
	if !bytes.HasPrefix(key, prefix) {
		return nil, false
	}

	remainder := key[len(prefix):]

	if len(remainder) == 0 {
		return nil, false
	}

	if remainder[0] == '_' {
		remainder = remainder[1:]
	}

	if len(remainder) == 0 {
		return nil, false
	}

	underscoreIndex := bytes.IndexByte(remainder, '_')

	if underscoreIndex >= 0 {
		return remainder[:underscoreIndex], true
	}

	return remainder, true
}

func predictionIndexForToken(
	predictions []LookaheadPrediction,
	token []byte,
) int {
	for index, prediction := range predictions {
		if bytes.Equal(prediction.Token, token) {
			return index
		}
	}

	return -1
}
