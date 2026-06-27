package dmt

import (
	"bytes"
	"errors"
	"math"
)

var (
	ErrEmptySequence    = errors.New("dmt: empty sequence")
	ErrNoAttractorMatch = errors.New("dmt: no attractor basin matched sequence")
)

const classificationClassCapacity = 16

/*
ClassScore records one posterior class probability after softmax normalization.
*/
type ClassScore struct {
	ClassName []byte
	Value     float64
}

/*
ClassificationResult is the sorted posterior matrix for one input sequence.
*/
type ClassificationResult struct {
	Scores  []ClassScore
	Winner  []byte
	Highest float64
}

/*
ClassificationScratch holds reusable stack storage for zero-allocation classification.
*/
type ClassificationScratch struct {
	scores      [classificationClassCapacity]ClassScore
	nameStorage [classificationClassCapacity][48]byte
	activeCount int
}

/*
Reset clears accumulated class evidence in the scratch buffer.
*/
func (scratch *ClassificationScratch) Reset() {
	scratch.activeCount = 0
}

/*
Classify evaluates a sensory sequence against attractor basins and returns posteriors.
*/
func (tree *Tree) Classify(
	sequence []byte,
	scratch *ClassificationScratch,
) ClassificationResult {
	if tree == nil || scratch == nil {
		return ClassificationResult{}
	}

	scratch.Reset()

	var keyScratch [128]byte
	root := tree.loadRoot()
	iterator := root.Root().Iterator()

	iterator.SeekPrefix(basinNamespaceBytes)

	for key, value, ok := iterator.Next(); ok; key, value, ok = iterator.Next() {
		if !bytes.HasPrefix(key, basinNamespaceBytes) {
			break
		}

		className, basinSequence, mapped := classSequenceFromBasinKey(key)

		if !mapped || !basinMatchesSequence(basinSequence, sequence) {
			continue
		}

		weight := UnmarshalCognitive(value)
		parentPath := parentContextPath(basinSequence)
		parentWeight := tree.getBasinWeightStack(className, parentPath, &keyScratch)
		probabilityFloor := probabilityFloorFromWeight(weight, parentWeight.Count)
		logEvidence := math.Log(math.Max(weight.Probability, probabilityFloor))

		scratch.accumulateClassEvidence(className, logEvidence)
	}

	if scratch.activeCount == 0 {
		return ClassificationResult{}
	}

	scores := scratch.scores[:scratch.activeCount]
	normalizeLogEvidence(scores)
	sortClassScoresDescending(scores)

	return ClassificationResult{
		Scores:  scores,
		Winner:  scores[0].ClassName,
		Highest: scores[0].Value,
	}
}

/*
UnsupervisedLearn infers the winning class and updates sensory and basin weights.
*/
func (tree *Tree) UnsupervisedLearn(
	sequence []byte,
	scratch *ClassificationScratch,
) ([]byte, float64, error) {
	if len(sequence) == 0 {
		return nil, 0, ErrEmptySequence
	}

	if tree == nil || scratch == nil {
		return nil, 0, ErrNoAttractorMatch
	}

	inference := tree.Classify(sequence, scratch)

	if len(inference.Winner) == 0 {
		return nil, 0, ErrNoAttractorMatch
	}

	learningRate := deriveLearningRate(tree, sequence)
	mutations := tree.buildUnsupervisedMutations(sequence, inference.Winner, learningRate)

	tree.commitLearnMutations(mutations)

	return inference.Winner, inference.Highest, nil
}

/*
optimizeWeightsInline runs unsupervised clustering during REM replay.
*/
func (tree *Tree) optimizeWeightsInline(
	sequence []byte,
	scratch *ClassificationScratch,
) error {
	_, _, err := tree.UnsupervisedLearn(sequence, scratch)

	if errors.Is(err, ErrNoAttractorMatch) {
		return nil
	}

	return err
}

func (scratch *ClassificationScratch) accumulateClassEvidence(
	className []byte,
	logEvidence float64,
) {
	for index := 0; index < scratch.activeCount; index++ {
		if bytes.Equal(scratch.scores[index].ClassName, className) {
			scratch.scores[index].Value += logEvidence

			return
		}
	}

	if scratch.activeCount >= classificationClassCapacity {
		return
	}

	nameBuffer := scratch.nameStorage[scratch.activeCount][:len(className)]
	copy(nameBuffer, className)

	scratch.scores[scratch.activeCount] = ClassScore{
		ClassName: nameBuffer,
		Value:     logEvidence,
	}
	scratch.activeCount++
}

func normalizeLogEvidence(scores []ClassScore) {
	logPeak := scores[0].Value

	for index := 1; index < len(scores); index++ {
		if scores[index].Value > logPeak {
			logPeak = scores[index].Value
		}
	}

	exponentialMass := 0.0

	for index := range scores {
		scores[index].Value = math.Exp(scores[index].Value - logPeak)
		exponentialMass += scores[index].Value
	}

	if exponentialMass <= 0 {
		return
	}

	for index := range scores {
		scores[index].Value /= exponentialMass
	}
}

func (tree *Tree) getBasinWeightStack(
	className []byte,
	sequence []byte,
	keyScratch *[128]byte,
) CognitiveState {
	requiredLength := len(basinNamespaceBytes) + len(className) + 1 + len(sequence)

	if requiredLength > len(keyScratch) {
		return CognitiveState{}
	}

	storageKey := keyScratch[:requiredLength]
	offset := copy(storageKey, basinNamespaceBytes)
	offset += copy(storageKey[offset:], className)
	storageKey[offset] = '/'
	copy(storageKey[offset+1:], sequence)

	value, found := tree.Get(storageKey)

	if !found {
		return CognitiveState{}
	}

	return UnmarshalCognitive(value)
}

func sortClassScoresDescending(scores []ClassScore) {
	for index := 1; index < len(scores); index++ {
		currentScore := scores[index]
		previousIndex := index - 1

		for previousIndex >= 0 && scores[previousIndex].Value < currentScore.Value {
			scores[previousIndex+1] = scores[previousIndex]
			previousIndex--
		}

		scores[previousIndex+1] = currentScore
	}
}

func basinMatchesSequence(basinSequence, sequence []byte) bool {
	if len(basinSequence) == 0 {
		return len(sequence) == 0
	}

	if bytes.HasPrefix(sequence, basinSequence) {
		if len(sequence) == len(basinSequence) {
			return true
		}

		return sequence[len(basinSequence)] == '_'
	}

	return bytes.HasPrefix(basinSequence, sequence)
}

func deriveLearningRate(tree *Tree, sequence []byte) float64 {
	surprisalSum := 0.0
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
		weight := tree.GetSensoryWeight(currentPath)
		parentPath := parentContextPath(currentPath)
		surprisalSum += tree.surprisalForWeight(weight, parentPath)
		tokenStart = index + 1
	}

	if surprisalSum <= 0 {
		tokenCount := countTokenBoundaries(sequence)

		return 1.0 / float64(tokenCount+1)
	}

	return 1.0 / (1.0 + surprisalSum)
}

type learnMutation struct {
	key   []byte
	value []byte
}

func (tree *Tree) buildUnsupervisedMutations(
	sequence []byte,
	inferredClass []byte,
	learningRate float64,
) []learnMutation {
	mutations := make([]learnMutation, 0, countTokenBoundaries(sequence)*2)
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
		sensoryKey := sensoryStorageKey(currentPath)
		sensoryWeight := tree.GetSensoryWeight(currentPath)
		sensoryWeight.Count++
		sensoryWeight.Probability = onlineProbabilityAlignment(
			sensoryWeight.Probability,
			learningRate,
		)

		mutations = append(mutations, learnMutation{
			key:   sensoryKey,
			value: MarshalCognitive(sensoryWeight),
		})

		basinKey := basinStorageKey(inferredClass, currentPath)
		basinWeight := tree.GetAttractorBasin(inferredClass, currentPath)
		basinWeight.Count++
		basinWeight.Probability = onlineProbabilityAlignment(
			basinWeight.Probability,
			learningRate,
		)

		mutations = append(mutations, learnMutation{
			key:   basinKey,
			value: MarshalCognitive(basinWeight),
		})

		tokenStart = index + 1
	}

	return mutations
}

func onlineProbabilityAlignment(currentProbability, learningRate float64) float64 {
	return currentProbability + learningRate*(1.0-currentProbability)
}

func (tree *Tree) commitLearnMutations(mutations []learnMutation) {
	if tree == nil || len(mutations) == 0 {
		return
	}

	for {
		oldRoot := tree.loadRoot()
		transaction := oldRoot.Txn()

		for _, mutation := range mutations {
			transaction.Insert(mutation.key, mutation.value)
		}

		newRoot := transaction.Commit()

		if tree.root.CompareAndSwap(oldRoot, newRoot) {
			tree.logLearnMutations(mutations)

			return
		}
	}
}

func (tree *Tree) logLearnMutations(mutations []learnMutation) {
	if tree.persist == nil {
		return
	}

	term := tree.term.Load()
	startIndex := tree.logIndex.Load()
	entries := make([]WALEntry, 0, len(mutations))

	for _, mutation := range mutations {
		index := startIndex + uint64(len(entries)) + 1
		entries = append(entries, WALEntry{
			Op:    opInsert,
			Term:  term,
			Index: index,
			Key:   mutation.key,
			Value: mutation.value,
		})
	}

	guardStep(tree.state, func() error {
		return tree.persist.LogInserts(entries)
	})

	if tree.state.Failed() {
		return
	}

	lastIndex := startIndex + uint64(len(entries))
	tree.logIndex.Store(lastIndex)

	if startIndex/tree.persist.snapCount != lastIndex/tree.persist.snapCount {
		guardStep(tree.state, tree.SaveSnapshot)
	}
}
