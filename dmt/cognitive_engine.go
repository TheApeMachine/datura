package dmt

import (
	"bytes"
	"math"
	"math/rand"
	"sort"
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
	value, found := tree.Get(sensoryStorageKey(sequence))

	if !found {
		return CognitiveState{}
	}

	return UnmarshalCognitive(value)
}

/*
InsertSensoryWeight writes s/[sequence] suffix statistics.
*/
func (tree *Tree) InsertSensoryWeight(sequence []byte, state CognitiveState) (*Tree, bool) {
	return tree.Insert(sensoryStorageKey(sequence), MarshalCognitive(state))
}

/*
GetAttractorBasin reads b/[class]/[sequence] posterior weights.
*/
func (tree *Tree) GetAttractorBasin(class []byte, sequence []byte) CognitiveState {
	value, found := tree.Get(basinStorageKey(class, sequence))

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
) (*Tree, bool) {
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
	targetBuffer = targetBuffer[:0]
	root := tree.loadRoot()
	iterator := root.Root().Iterator()

	iterator.SeekPrefix(storagePrefix)

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

/*
SelectStochasticToken applies temperature-scaled softmax over candidate scores.
At zero temperature it selects the highest-scoring token deterministically.
*/
func SelectStochasticToken(candidates []CandidateToken, temperature float64) []byte {
	if len(candidates) == 0 {
		return nil
	}

	if len(candidates) == 1 || temperature <= 0 {
		return highestScoreCandidate(candidates).Token
	}

	scoreFloor := positiveScoreFloor(candidates)
	effectiveTemperature := math.Max(temperature, scoreFloor)

	exponentialScores := make([]float64, len(candidates))
	totalMass := 0.0

	for index, candidate := range candidates {
		scaledLogit := math.Log(math.Max(candidate.Score, scoreFloor)) / effectiveTemperature
		exponentialScores[index] = math.Exp(scaledLogit)
		totalMass += exponentialScores[index]
	}

	if totalMass <= 0 {
		return highestScoreCandidate(candidates).Token
	}

	threshold := rand.Float64() * totalMass
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

				scratch.NextBeams = append(scratch.NextBeams, BeamPath{
					Sequence: append([]byte(nil), scratch.PathBuffer...),
					Score:    beam.Score + logProbability,
				})
			}
		}

		if len(scratch.NextBeams) == 0 {
			break
		}

		sort.Slice(scratch.NextBeams, func(leftIndex, rightIndex int) bool {
			return scratch.NextBeams[leftIndex].Score > scratch.NextBeams[rightIndex].Score
		})

		if len(scratch.NextBeams) > beamWidth {
			scratch.NextBeams = scratch.NextBeams[:beamWidth]
		}

		scratch.CurrentBeams, scratch.NextBeams = scratch.NextBeams, scratch.CurrentBeams
	}

	results := make([]BeamPath, len(scratch.CurrentBeams))
	copy(results, scratch.CurrentBeams)

	return results
}

/*
CommitToEpisodicBuffer stores e/[timestamp][sequence] episodic observations.
*/
func (tree *Tree) CommitToEpisodicBuffer(timestamp uint64, sequence []byte) (*Tree, bool) {
	return tree.Insert(episodicStorageKey(timestamp, sequence), []byte{1})
}

/*
ExecuteREMSleepConsolidation replays episodic entries, retrains sensory weights,
and applies retroactive inhibition across the sensory namespace.
*/
func (tree *Tree) ExecuteREMSleepConsolidation(startTimestamp, endTimestamp uint64) {
	replayedObservations := uint64(0)

	tree.WalkPrefix([]byte(episodicNamespace), func(storageKey, value []byte) bool {
		timestamp, sequence, mapped := timestampFromEpisodicKey(storageKey)

		if !mapped {
			return true
		}

		if timestamp < startTimestamp {
			return true
		}

		if timestamp > endTimestamp {
			return false
		}

		if len(value) == 0 {
			return true
		}

		tree.TrainSensorySequence(sequence)
		replayedObservations++

		return true
	})

	namespaceEntries := uint64(countNamespaceEntries(tree, []byte(sensoryNamespace)))
	decayFactor := deriveDecayFactor(replayedObservations, namespaceEntries)

	tree.ExecuteDecayConsolidation([]byte(sensoryNamespace), decayFactor)
}

/*
TrainSensorySequence increments sensory counts and conditional probabilities inline.
*/
func (tree *Tree) TrainSensorySequence(sequence []byte) {
	tokenStart := 0

	for index := 0; index <= len(sequence); index++ {
		if index < len(sequence) && sequence[index] != '_' {
			continue
		}

		if index == tokenStart {
			tokenStart = index + 1

			continue
		}

		currentPath := sequence[:index]
		parentPath := parentContextPath(currentPath)
		currentState := tree.GetSensoryWeight(currentPath)
		parentState := tree.GetSensoryWeight(parentPath)

		nextCount := currentState.Count + 1
		probability := 1.0

		if len(parentPath) > 0 {
			denominator := float64(parentState.Count + nextCount)

			if denominator <= 0 {
				denominator = float64(nextCount)
			}

			probability = float64(nextCount) / denominator
		}

		_, _ = tree.InsertSensoryWeight(currentPath, CognitiveState{
			Count:       nextCount,
			Probability: probability,
		})

		tokenStart = index + 1
	}
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

func positiveScoreFloor(candidates []CandidateToken) float64 {
	floor := math.MaxFloat64

	for _, candidate := range candidates {
		if candidate.Score <= 0 {
			continue
		}

		if candidate.Score < floor {
			floor = candidate.Score
		}
	}

	if floor == math.MaxFloat64 {
		return math.SmallestNonzeroFloat64
	}

	return floor
}
