package dmt

import (
	"bytes"
	"math"
	"math/rand"
)

/*
CandidateToken is one branch option for thermodynamic selection.
*/
type CandidateToken struct {
	Token []byte
	Score float64
}

/*
BeamPath is one scored multi-hop prefix path.
*/
type BeamPath struct {
	Sequence []byte
	Score    float64
}

/*
BeamSearchScratch holds reusable buffers for beam search iterations.
*/
type BeamSearchScratch struct {
	CurrentBeams []BeamPath
	NextBeams    []BeamPath
	LookupBuffer []LookaheadPrediction
	PathBuffer   []byte
}

/*
GetSensoryWeight reads s/[sequence] suffix statistics.
*/
func (tree *Tree) GetSensoryWeight(sequence []byte) CognitiveState {
	value, found := tree.getRaw(sensoryStorageKey(sequence))

	if !found {
		return CognitiveState{}
	}

	return UnmarshalCognitive(value)
}

/*
InsertSensoryWeight writes s/[sequence] suffix statistics.
*/
func (tree *Tree) InsertSensoryWeight(sequence []byte, state CognitiveState) (*Tree, bool, error) {
	updated, changed, err := tree.Insert(
		sensoryStorageKey(sequence),
		MarshalCognitive(state),
	)

	if changed {
		tree.maintainCognitiveIndexes(sensoryStorageKey(sequence), false)
	}

	return updated, changed, err
}

/*
GetAttractorBasin reads b/[class]/[sequence] posterior weights.
*/
func (tree *Tree) GetAttractorBasin(class []byte, sequence []byte) CognitiveState {
	value, found := tree.getRaw(basinStorageKey(class, sequence))

	if !found {
		return CognitiveState{}
	}

	return UnmarshalCognitive(value)
}

/*
InsertAttractorBasin writes b/[class]/[sequence] posterior weights.
*/
func (tree *Tree) InsertAttractorBasin(
	class []byte,
	sequence []byte,
	state CognitiveState,
) (*Tree, bool, error) {
	return tree.Insert(basinStorageKey(class, sequence), MarshalCognitive(state))
}

/*
PredictNextSensoryTokens performs lookahead on the sensory namespace.
*/
func (tree *Tree) PredictNextSensoryTokens(
	sequencePrefix []byte,
	targetBuffer []LookaheadPrediction,
) []LookaheadPrediction {
	return tree.predictNextTokensOnPrefix(
		sensoryStorageKey(sequencePrefix),
		sequencePrefix,
		targetBuffer,
	)
}

func (tree *Tree) predictNextTokensOnPrefix(
	storagePrefix []byte,
	sequencePrefix []byte,
	targetBuffer []LookaheadPrediction,
) []LookaheadPrediction {
	limit := cap(targetBuffer)

	if limit <= 0 {
		limit = defaultChildIndexCapacity
	}

	if indexed, ok := tree.readChildProbabilityIndex(storagePrefix, targetBuffer[:0], limit); ok {
		return indexed
	}

	targetBuffer = targetBuffer[:0]
	root := tree.loadRoot()
	iterator := root.Root().Iterator()

	iterator.SeekPrefix(storagePrefix)

	truncated := false

	for key, value, ok := iterator.Next(); ok; key, value, ok = iterator.Next() {
		if !bytes.HasPrefix(key, storagePrefix) {
			break
		}

		sequenceKey, mapped := sequenceFromSensoryKey(key)

		if !mapped {
			continue
		}

		tokenSuffix, isChild := immediateTokenSuffix(sequencePrefix, sequenceKey)

		if !isChild {
			continue
		}

		weight := UnmarshalCognitive(value)
		targetBuffer, truncated = insertTopKPrediction(
			targetBuffer,
			tokenSuffix,
			weight.Probability,
			limit,
		)
	}

	if len(targetBuffer) > 0 {
		tree.storeChildProbabilityIndex(storagePrefix, targetBuffer, truncated)
	}

	return targetBuffer
}

/*
SelectStochasticToken applies temperature-scaled softmax over candidate scores.
At zero temperature it selects the highest-scoring token deterministically.
source must be a non-nil RNG so selection is reproducible per engine/test.
*/
func SelectStochasticToken(
	candidates []CandidateToken,
	temperature float64,
	source *rand.Rand,
) []byte {
	if len(candidates) == 0 {
		return nil
	}

	if temperature <= 0 || len(candidates) == 1 {
		return highestScoreCandidate(candidates).Token
	}

	if source == nil {
		source = rand.New(rand.NewSource(1))
	}

	exponentialScores := make([]float64, len(candidates))
	probabilityMass := 0.0

	for index, candidate := range candidates {
		exponentialScores[index] = math.Exp(candidate.Score / temperature)
		probabilityMass += exponentialScores[index]
	}

	if probabilityMass <= 0 {
		return highestScoreCandidate(candidates).Token
	}

	threshold := source.Float64() * probabilityMass
	runningMass := 0.0

	for index, exponentialScore := range exponentialScores {
		runningMass += exponentialScore

		if threshold <= runningMass {
			return candidates[index].Token
		}
	}

	return highestScoreCandidate(candidates).Token
}

/*
ExecuteBeamSearch explores multi-hop sensory paths using log-probability scoring.
Expansions keep a bounded top-k beam without sorting the full candidate list when
possible; parent sequences are reused through scratch path buffers.
*/
func (tree *Tree) ExecuteBeamSearch(
	contextPrefix []byte,
	beamWidth int,
	maxHops int,
	scratch *BeamSearchScratch,
) []BeamPath {
	if beamWidth <= 0 || maxHops <= 0 {
		return nil
	}

	if scratch == nil {
		scratch = &BeamSearchScratch{}
	}

	if cap(scratch.CurrentBeams) < 1 {
		scratch.CurrentBeams = make([]BeamPath, 0, beamWidth)
	}

	if cap(scratch.NextBeams) < beamWidth {
		scratch.NextBeams = make([]BeamPath, 0, beamWidth)
	}

	scratch.CurrentBeams = scratch.CurrentBeams[:0]
	scratch.CurrentBeams = append(scratch.CurrentBeams, BeamPath{
		Sequence: append([]byte(nil), contextPrefix...),
		Score:    0,
	})

	logFloor := math.SmallestNonzeroFloat64

	for hop := 0; hop < maxHops; hop++ {
		scratch.NextBeams = scratch.NextBeams[:0]

		for _, beam := range scratch.CurrentBeams {
			scratch.LookupBuffer = tree.PredictNextSensoryTokens(beam.Sequence, scratch.LookupBuffer[:0])

			if len(scratch.LookupBuffer) == 0 {
				continue
			}

			for _, prediction := range scratch.LookupBuffer {
				scratch.PathBuffer = appendSequenceToken(scratch.PathBuffer[:0], beam.Sequence, prediction.Token)
				logProbability := math.Log(math.Max(prediction.Probability, logFloor))
				candidate := BeamPath{
					Sequence: append([]byte(nil), scratch.PathBuffer...),
					Score:    beam.Score + logProbability,
				}
				scratch.NextBeams = insertBeamTopK(scratch.NextBeams, candidate, beamWidth)
			}
		}

		if len(scratch.NextBeams) == 0 {
			break
		}

		scratch.CurrentBeams, scratch.NextBeams = scratch.NextBeams, scratch.CurrentBeams
	}

	results := make([]BeamPath, len(scratch.CurrentBeams))
	copy(results, scratch.CurrentBeams)

	return results
}

/*
insertBeamTopK inserts candidate into a descending-score beam capped at width.
*/
func insertBeamTopK(beams []BeamPath, candidate BeamPath, width int) []BeamPath {
	insertAt := len(beams)

	for index := range beams {
		if candidate.Score > beams[index].Score {
			insertAt = index
			break
		}
	}

	if insertAt >= width {
		return beams
	}

	if len(beams) < width {
		beams = append(beams, BeamPath{})
	}

	copy(beams[insertAt+1:], beams[insertAt:])
	beams[insertAt] = candidate

	if len(beams) > width {
		beams = beams[:width]
	}

	return beams
}

/*
TrainSensorySequence increments sensory counts and conditional probabilities inline.
*/
func (tree *Tree) TrainSensorySequence(sequence []byte) {
	tree.TrainSensorySequenceAt(0, sequence)
}

/*
TrainSensorySequenceAt stamps LastObserved with the supplied event time.
*/
func (tree *Tree) TrainSensorySequenceAt(observedAt uint64, sequence []byte) {
	_ = tree.commitLearnMutations(tree.buildSensoryMutations(observedAt, sequence))
}

func (tree *Tree) buildSensoryMutations(observedAt uint64, sequence []byte) []learnMutation {
	if tree == nil || len(sequence) == 0 {
		return nil
	}

	mutations := make([]learnMutation, 0, countTokenBoundaries(sequence))
	pending := make(map[string]CognitiveState, countTokenBoundaries(sequence))
	tokenStart := 0

	for index := 0; index <= len(sequence); index++ {
		if index < len(sequence) && sequence[index] != '_' {
			continue
		}

		if index == tokenStart {
			tokenStart = index + 1

			continue
		}

		currentPath := append([]byte(nil), sequence[:index]...)
		parentPath := parentContextPath(currentPath)
		currentState := tree.GetSensoryWeight(currentPath)
		parentState := tree.GetSensoryWeight(parentPath)

		if pendingCurrent, ok := pending[string(currentPath)]; ok {
			currentState = pendingCurrent
		}
		if pendingParent, ok := pending[string(parentPath)]; ok {
			parentState = pendingParent
		}

		nextCount := currentState.Count + 1
		probability := 1.0

		if len(parentPath) > 0 {
			denominator := float64(parentState.Count)

			if denominator <= 0 {
				denominator = float64(nextCount)
			}

			probability = float64(nextCount) / denominator
			if probability > 1.0 {
				probability = 1.0
			}
		}

		next := CognitiveState{
			Count:        nextCount,
			Probability:  probability,
			LastObserved: observedAt,
		}
		pending[string(currentPath)] = next
		mutations = append(mutations, learnMutation{
			key:   sensoryStorageKey(currentPath),
			value: MarshalCognitive(next),
		})

		tokenStart = index + 1
	}

	return mutations
}

func appendSequenceToken(buffer []byte, prefix []byte, token []byte) []byte {
	if len(prefix) == 0 {
		return append(buffer, token...)
	}

	buffer = append(buffer, prefix...)
	buffer = append(buffer, '_')

	return append(buffer, token...)
}

func highestScoreCandidate(candidates []CandidateToken) CandidateToken {
	bestCandidate := candidates[0]

	for _, candidate := range candidates[1:] {
		if candidate.Score > bestCandidate.Score {
			bestCandidate = candidate
		}
	}

	return bestCandidate
}
