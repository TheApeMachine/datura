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
Truncated is true when the scan stopped early under a capacity limit.
*/
type AnalogyMatch struct {
	ClosestKey []byte
	Score      int
	Truncated  bool
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
FindStructuralAnalog scans keys within the unknown key's namespace prefix for the
longest shared prefix. Truncation is reported when the candidate cap is hit.
*/
func (tree *Tree) FindStructuralAnalog(unknownKey []byte) (AnalogyMatch, bool) {
	candidates, indexTruncated := tree.collectAnalogCandidates(unknownKey)
	bestMatch := AnalogyMatch{Score: -1}
	minimumScore := analogMinimumScore(unknownKey)

	if len(candidates) > 0 {
		for _, candidate := range candidates {
			matchLength := sharedPrefixLength(candidate, unknownKey)

			if matchLength <= bestMatch.Score {
				continue
			}

			bestMatch.Score = matchLength
			bestMatch.ClosestKey = append(bestMatch.ClosestKey[:0], candidate...)
		}

		bestMatch.Truncated = indexTruncated

		if bestMatch.Score >= minimumScore {
			return bestMatch, true
		}
	}

	root := tree.loadRoot()
	iterator := root.Root().Iterator()

	namespace := namespacePrefixOf(unknownKey)
	iterator.SeekPrefix(namespace)

	const candidateCap = 4096
	examined := 0

	for key, _, ok := iterator.Next(); ok; key, _, ok = iterator.Next() {
		if len(namespace) > 0 && !bytes.HasPrefix(key, namespace) {
			break
		}

		examined++

		if examined > candidateCap {
			bestMatch.Truncated = true
			break
		}

		matchLength := sharedPrefixLength(key, unknownKey)

		if matchLength <= bestMatch.Score {
			continue
		}

		bestMatch.Score = matchLength
		bestMatch.ClosestKey = append(bestMatch.ClosestKey[:0], key...)
	}

	if bestMatch.Score < minimumScore {
		return AnalogyMatch{Truncated: bestMatch.Truncated}, false
	}

	return bestMatch, true
}

func namespacePrefixOf(key []byte) []byte {
	index := bytes.IndexByte(key, '/')

	if index <= 0 {
		return nil
	}

	return key[:index+1]
}

/*
CompareSensoryBranches measures token-sequence overlap between two sensory prefixes.
*/
func (tree *Tree) CompareSensoryBranches(leftPrefix, rightPrefix []byte) float64 {
	return tokenSequenceSimilarity(leftPrefix, rightPrefix)
}

/*
ExecuteDecayConsolidation degrades stale namespace weights and prunes dead branches.
Preserved sequences skip decay so freshly replayed REM paths are retained.
When observedAt is non-zero, decay follows elapsed event time instead of a ratio.
*/
func (tree *Tree) ExecuteDecayConsolidation(
	namespacePrefix []byte,
	decayFactor float64,
	preservedSequences ...[]byte,
) {
	if tree == nil || decayFactor <= 0 {
		return
	}

	tree.applyDecayConsolidation(
		namespacePrefix,
		decayFactor,
		0,
		countNamespaceEntries(tree, namespacePrefix),
		preservedSequences...,
	)
}

/*
applyDecayConsolidation streams namespace rewrites in persistence-quantum batches
and compacts via one forced snapshot. Logging every decayed key into the WAL
materializes the full sensory namespace twice and is what froze long REM runs.
*/
func (tree *Tree) applyDecayConsolidation(
	namespacePrefix []byte,
	decayFactor float64,
	observedAt uint64,
	namespaceEntries int,
	preservedSequences ...[]byte,
) {
	if tree == nil || namespaceEntries <= 0 {
		return
	}

	if decayFactor <= 0 && observedAt == 0 {
		return
	}

	oldRoot := tree.loadRoot()
	iterator := oldRoot.Root().Iterator()
	iterator.SeekPrefix(namespacePrefix)

	preserved := make(map[string]struct{}, len(preservedSequences))

	for _, sequence := range preservedSequences {
		preserved[string(sequence)] = struct{}{}
	}

	batchLimit := tree.decayBatchLimit()
	batch := make([]decayMutation, 0, batchLimit)
	mutated := false

	flush := func() {
		if len(batch) == 0 {
			return
		}

		tree.commitDecayMutations(batch)
		mutated = true
		batch = batch[:0]
	}

	for key, value, ok := iterator.Next(); ok; key, value, ok = iterator.Next() {
		if !bytes.HasPrefix(key, namespacePrefix) {
			break
		}

		if sensoryKeyPreservedSet(key, preserved) {
			continue
		}

		weight := UnmarshalCognitive(value)

		if observedAt > 0 {
			weight.Probability *= eventTimeDecayMultiplier(
				weight.LastObserved,
				observedAt,
				weight.Count,
				namespaceEntries,
			)
		} else {
			weight.Probability *= decayFactor
		}

		entryPruneThreshold := pruneProbabilityThreshold(namespaceEntries, weight)

		if weight.Probability < entryPruneThreshold {
			batch = append(batch, decayMutation{
				key:    append([]byte(nil), key...),
				delete: true,
			})
		} else {
			batch = append(batch, decayMutation{
				key:   append([]byte(nil), key...),
				value: MarshalCognitive(weight),
			})
		}

		if len(batch) >= batchLimit {
			flush()
		}
	}

	flush()

	if mutated {
		tree.invalidateDerivedIndexes(namespacePrefix)
		_ = tree.SaveSnapshotForced()
	}
}

/*
decayBatchLimit follows the persistent store's snapshot quantum so REM decay
never holds a full-namespace mutation list in memory.
*/
func (tree *Tree) decayBatchLimit() int {
	if tree != nil && tree.persist != nil && tree.persist.snapCount > 0 {
		return int(tree.persist.snapCount)
	}

	return 1000
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
			return
		}
	}
}

/*
invalidateDerivedIndexes drops namespace-scoped child indexes after bulk decay.
Analog indexes are rebuilt lazily on the next insert.
*/
func (tree *Tree) invalidateDerivedIndexes(namespacePrefix []byte) {
	if tree == nil || len(namespacePrefix) == 0 {
		return
	}

	indexPrefix := append([]byte(childIndexNamespace), namespacePrefix...)
	keysToDelete := make([][]byte, 0)

	tree.WalkPrefix(indexPrefix, func(key, value []byte) bool {
		keysToDelete = append(keysToDelete, append([]byte(nil), key...))

		return true
	})

	for _, key := range keysToDelete {
		tree.deleteEphemeral(key)
	}
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

func pruneProbabilityThreshold(namespaceEntries int, weight CognitiveState) float64 {
	if namespaceEntries <= 0 {
		return math.SmallestNonzeroFloat64
	}

	namespaceMass := float64(namespaceEntries)
	countMass := float64(weight.Count) + 1.0

	return 1.0 / (namespaceMass * countMass)
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

/*
eventTimeDecayMultiplier scales probability by elapsed event time relative to the
observation's own count mass and the active namespace size.
*/
func eventTimeDecayMultiplier(
	lastObserved, observedAt uint64,
	observationCount uint64,
	namespaceEntries int,
) float64 {
	if observedAt <= lastObserved {
		return 1.0
	}

	elapsed := float64(observedAt - lastObserved)
	scale := float64(observationCount) + float64(namespaceEntries) + 1.0

	return scale / (scale + elapsed)
}

func sensoryKeyPreservedSet(storageKey []byte, preserved map[string]struct{}) bool {
	sequence, mapped := sequenceFromSensoryKey(storageKey)

	if !mapped {
		return false
	}

	_, ok := preserved[string(sequence)]
	return ok
}

func sensoryPrefixPaths(sequence []byte) [][]byte {
	if len(sequence) == 0 {
		return nil
	}

	tokenStart := 0
	paths := make([][]byte, 0, countTokenBoundaries(sequence))

	for index := 0; index <= len(sequence); index++ {
		if index < len(sequence) && sequence[index] != '_' {
			continue
		}

		if index == tokenStart {
			tokenStart = index + 1

			continue
		}

		paths = append(paths, append([]byte(nil), sequence[:index]...))
		tokenStart = index + 1
	}

	return paths
}

func tokenSequenceSimilarity(leftSequence, rightSequence []byte) float64 {
	leftTokens := splitUnderscoreTokens(leftSequence)
	rightTokens := splitUnderscoreTokens(rightSequence)

	if len(leftTokens) == 0 || len(rightTokens) == 0 {
		return 0
	}

	sharedDepth := 0
	pairCount := min(len(leftTokens), len(rightTokens))

	for index := range pairCount {
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
