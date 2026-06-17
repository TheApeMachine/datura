package dmt

import (
	"bytes"
	"math"
)

/*
ContrastiveEvidence records localized information-theoretic routing evidence.
*/
type ContrastiveEvidence struct {
	WinnerBits   float64
	RunnerUpBits float64
	Divergence   float64
}

/*
AnalogyMatch is the closest topological sibling for an unknown key.
*/
type AnalogyMatch struct {
	ClosestKey []byte
	Score      int
}

/*
AmbiguityState captures branch entropy relative to a uniform split baseline.
*/
type AmbiguityState struct {
	Prefix      []byte
	EntropyBits float64
	Threshold   float64
	Ambiguous   bool
}

/*
decayMutation records one radix mutation produced by decay consolidation.
*/
type decayMutation struct {
	key    []byte
	value  []byte
	delete bool
}

/*
ComputeContrastiveEvidence calculates surprisal and localized KL divergence
between two competing context paths using dynamically derived probability floors.
*/
func (tree *Tree) ComputeContrastiveEvidence(
	winnerPath, runnerUpPath []byte,
) ContrastiveEvidence {
	winnerWeight := tree.GetContextWeight(winnerPath)
	runnerUpWeight := tree.GetContextWeight(runnerUpPath)

	winnerParent := parentContextPath(winnerPath)
	runnerParent := parentContextPath(runnerUpPath)

	winnerFloor := probabilityFloorFromWeight(
		winnerWeight,
		tree.GetContextWeight(winnerParent).Count,
	)
	runnerFloor := probabilityFloorFromWeight(
		runnerUpWeight,
		tree.GetContextWeight(runnerParent).Count,
	)

	pWinner := math.Max(winnerWeight.Probability, winnerFloor)
	pRunnerUp := math.Max(runnerUpWeight.Probability, runnerFloor)

	winnerBits := -math.Log2(pWinner)
	runnerUpBits := -math.Log2(pRunnerUp)
	klDivergence := pWinner * math.Log2(pWinner/pRunnerUp)

	return ContrastiveEvidence{
		WinnerBits:   winnerBits,
		RunnerUpBits: runnerUpBits,
		Divergence:   klDivergence,
	}
}

/*
ComputeBasinContrastiveEvidence contrasts two attractor basin posteriors.
*/
func (tree *Tree) ComputeBasinContrastiveEvidence(
	winnerClass, runnerUpClass, sequence []byte,
) ContrastiveEvidence {
	winnerPath := basinStorageKey(winnerClass, sequence)
	runnerPath := basinStorageKey(runnerUpClass, sequence)

	return tree.ComputeContrastiveEvidence(winnerPath, runnerPath)
}

/*
CalculateBranchEntropy computes Shannon entropy across immediate child branches.
*/
func (tree *Tree) CalculateBranchEntropy(prefix []byte) float64 {
	var predictions [32]LookaheadPrediction

	buffer := tree.PredictNextTokens(prefix, predictions[:0])

	if len(buffer) <= 1 {
		return 0
	}

	probabilityMass := 0.0

	for _, prediction := range buffer {
		probabilityMass += prediction.Probability
	}

	if probabilityMass <= 0 {
		return 0
	}

	entropyBits := 0.0

	for _, prediction := range buffer {
		normalizedProbability := prediction.Probability / probabilityMass

		if normalizedProbability <= 0 {
			continue
		}

		entropyBits -= normalizedProbability * math.Log2(normalizedProbability)
	}

	return entropyBits
}

/*
MeasureBranchAmbiguity evaluates whether a prefix exceeds its uniform entropy baseline.
*/
func (tree *Tree) MeasureBranchAmbiguity(prefix []byte) AmbiguityState {
	var predictions [32]LookaheadPrediction

	buffer := tree.PredictNextTokens(prefix, predictions[:0])
	entropyBits := tree.CalculateBranchEntropy(prefix)
	branchCount := len(buffer)

	parentState := tree.GetContextWeight(prefix)
	threshold := ambiguityEntropyThreshold(branchCount, parentState)

	return AmbiguityState{
		Prefix:      append([]byte(nil), prefix...),
		EntropyBits: entropyBits,
		Threshold:   threshold,
		Ambiguous:   branchCount > 1 && entropyBits >= threshold,
	}
}

/*
FindStructuralAnalog scans keys for the longest shared prefix with unknownKey.
*/
func (tree *Tree) FindStructuralAnalog(unknownKey []byte) (AnalogyMatch, bool) {
	root := tree.loadRoot()
	iterator := root.Root().Iterator()

	bestMatch := AnalogyMatch{Score: -1}
	minimumScore := analogMinimumScore(unknownKey)

	for key, _, ok := iterator.Next(); ok; key, _, ok = iterator.Next() {
		matchLength := sharedPrefixLength(key, unknownKey)

		if matchLength <= bestMatch.Score {
			continue
		}

		bestMatch.Score = matchLength
		bestMatch.ClosestKey = append(bestMatch.ClosestKey[:0], key...)
	}

	if bestMatch.Score < minimumScore {
		return AnalogyMatch{}, false
	}

	return bestMatch, true
}

/*
CompareSensoryBranches measures token-sequence overlap between two sensory prefixes.
*/
func (tree *Tree) CompareSensoryBranches(leftPrefix, rightPrefix []byte) float64 {
	return tokenSequenceSimilarity(leftPrefix, rightPrefix)
}

/*
ExecuteDecayConsolidation degrades stale namespace weights and prunes dead branches.
*/
func (tree *Tree) ExecuteDecayConsolidation(
	namespacePrefix []byte,
	decayFactor float64,
) {
	if tree == nil || decayFactor <= 0 {
		return
	}

	oldRoot := tree.loadRoot()
	iterator := oldRoot.Root().Iterator()
	iterator.SeekPrefix(namespacePrefix)

	namespaceEntries := countNamespaceEntries(tree, namespacePrefix)

	iterator = oldRoot.Root().Iterator()
	iterator.SeekPrefix(namespacePrefix)

	mutations := make([]decayMutation, 0, namespaceEntries)
	pruneThreshold := pruneProbabilityThreshold(namespaceEntries)

	for key, value, ok := iterator.Next(); ok; key, value, ok = iterator.Next() {
		if !bytes.HasPrefix(key, namespacePrefix) {
			break
		}

		weight := UnmarshalCognitive(value)
		weight.Probability *= decayFactor

		if weight.Probability < pruneThreshold {
			mutations = append(mutations, decayMutation{
				key:    append([]byte(nil), key...),
				delete: true,
			})

			continue
		}

		mutations = append(mutations, decayMutation{
			key:   append([]byte(nil), key...),
			value: MarshalCognitive(weight),
		})
	}

	tree.commitDecayMutations(mutations)
}

func (tree *Tree) commitDecayMutations(mutations []decayMutation) {
	if len(mutations) == 0 {
		return
	}

	for {
		oldRoot := tree.loadRoot()
		transaction := oldRoot.Txn()

		for _, mutation := range mutations {
			if mutation.delete {
				transaction.Delete(mutation.key)

				continue
			}

			transaction.Insert(mutation.key, mutation.value)
		}

		newRoot := transaction.Commit()

		if tree.root.CompareAndSwap(oldRoot, newRoot) {
			tree.logDecayMutations(mutations)

			return
		}
	}
}

func (tree *Tree) logDecayMutations(mutations []decayMutation) {
	if tree.persist == nil {
		return
	}

	term, index := tree.GetLogState()

	for _, mutation := range mutations {
		index++

		if mutation.delete {
			_ = tree.persist.LogDelete(mutation.key, term, index)

			continue
		}

		_ = tree.persist.LogInsert(mutation.key, mutation.value, term, index)
	}

	tree.logIndex.Store(index)
}

func countNamespaceEntries(tree *Tree, namespacePrefix []byte) int {
	entryCount := 0

	tree.WalkPrefix(namespacePrefix, func(key, value []byte) bool {
		entryCount++

		return true
	})

	return entryCount
}

func probabilityFloorFromWeight(weight PackedWeight, parentCount uint64) float64 {
	denominator := float64(weight.Count) + float64(parentCount) + 1.0

	if denominator <= 0 {
		return math.SmallestNonzeroFloat64
	}

	return 1.0 / denominator
}

func pruneProbabilityThreshold(namespaceEntries int) float64 {
	if namespaceEntries <= 0 {
		return math.SmallestNonzeroFloat64
	}

	return 1.0 / float64(namespaceEntries)
}

func ambiguityEntropyThreshold(branchCount int, parentState CognitiveState) float64 {
	if branchCount <= 1 {
		return math.MaxFloat64
	}

	uniformEntropy := math.Log2(float64(branchCount))
	parentUncertainty := 1.0 - parentState.Probability

	if parentState.Probability <= 0 {
		parentUncertainty = 1.0
	}

	return uniformEntropy * (1.0 - parentUncertainty/float64(branchCount))
}

func analogMinimumScore(unknownKey []byte) int {
	if len(unknownKey) == 0 {
		return 1
	}

	tokenBoundary := bytes.IndexByte(unknownKey, '_')

	if tokenBoundary < 0 {
		return (len(unknownKey) + 1) / 2
	}

	return tokenBoundary + 1
}

func sharedPrefixLength(leftKey, rightKey []byte) int {
	matchLength := 0
	maxLength := min(len(leftKey), len(rightKey))

	for matchLength < maxLength && leftKey[matchLength] == rightKey[matchLength] {
		matchLength++
	}

	return matchLength
}

func deriveDecayFactor(replayedObservations, namespaceEntries uint64) float64 {
	denominator := replayedObservations + namespaceEntries

	if denominator == 0 {
		return 1.0
	}

	return float64(replayedObservations) / float64(denominator)
}

func tokenSequenceSimilarity(leftSequence, rightSequence []byte) float64 {
	leftTokens := splitUnderscoreTokens(leftSequence)
	rightTokens := splitUnderscoreTokens(rightSequence)

	if len(leftTokens) == 0 || len(rightTokens) == 0 {
		return 0
	}

	sharedDepth := 0
	pairCount := min(len(leftTokens), len(rightTokens))

	for index := 0; index < pairCount; index++ {
		if !bytes.Equal(leftTokens[index], rightTokens[index]) {
			break
		}

		sharedDepth++
	}

	maxDepth := max(len(leftTokens), len(rightTokens))

	if maxDepth == 0 {
		return 0
	}

	return float64(sharedDepth) / float64(maxDepth)
}

func splitUnderscoreTokens(sequence []byte) [][]byte {
	if len(sequence) == 0 {
		return nil
	}

	tokenStart := 0
	tokens := make([][]byte, 0, countTokenBoundaries(sequence))

	for index := 0; index <= len(sequence); index++ {
		if index < len(sequence) && sequence[index] != '_' {
			continue
		}

		if index == tokenStart {
			tokenStart = index + 1

			continue
		}

		tokens = append(tokens, sequence[tokenStart:index])
		tokenStart = index + 1
	}

	return tokens
}
