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

const (
	classificationClassCapacity = 256
	classificationNameCapacity  = 256
)

/*
ClassScore records one posterior class probability after softmax normalization.
*/
type ClassScore struct {
	ClassName []byte
	Value     float64
}

/*
ClassificationResult is an owned, immutable posterior matrix for one input sequence.
*/
type ClassificationResult struct {
	Scores  []ClassScore
	Winner  []byte
	Highest float64
}

/*
ClassificationScratch holds reusable dynamically sized storage for classification.
Limits are explicit; exceeding them returns an error from Classify.
*/
type ClassificationScratch struct {
	scores      []ClassScore
	nameStorage [][]byte
	activeCount int
	classLimit  int
	nameLimit   int
}

/*
Reset clears accumulated class evidence in the scratch buffer.
*/
func (scratch *ClassificationScratch) Reset() {
	if scratch == nil {
		return
	}

	scratch.activeCount = 0

	if scratch.classLimit == 0 {
		scratch.classLimit = classificationClassCapacity
	}

	if scratch.nameLimit == 0 {
		scratch.nameLimit = classificationNameCapacity
	}
}

/*
Classify evaluates a sensory sequence against attractor basins and returns owned
posteriors. It matches only basins that are token-boundary prefixes of sequence.
*/
func (tree *Tree) Classify(
	sequence []byte,
	scratch *ClassificationScratch,
) (ClassificationResult, error) {
	if tree == nil || scratch == nil {
		return ClassificationResult{}, ErrNoAttractorMatch
	}

	scratch.Reset()

	var keyScratch [128]byte
	var prefixScratch [128]byte
	classes := tree.basinClasses()

	if len(classes) == 0 {
		if err := tree.classifyFullScan(sequence, scratch, &keyScratch); err != nil {
			return ClassificationResult{}, err
		}
	} else {
		for _, className := range classes {
			if err := tree.accumulateClassBasins(
				[]byte(className),
				sequence,
				scratch,
				&keyScratch,
				&prefixScratch,
			); err != nil {
				return ClassificationResult{}, err
			}
		}
	}

	if scratch.activeCount == 0 {
		return ClassificationResult{}, nil
	}

	scores := scratch.scores[:scratch.activeCount]
	normalizeLogEvidence(scores)
	sortClassScoresDescending(scores)

	owned := make([]ClassScore, len(scores))

	for index := range scores {
		owned[index] = ClassScore{
			ClassName: append([]byte(nil), scores[index].ClassName...),
			Value:     scores[index].Value,
		}
	}

	return ClassificationResult{
		Scores:  owned,
		Winner:  append([]byte(nil), owned[0].ClassName...),
		Highest: owned[0].Value,
	}, nil
}

/*
classifyFullScan is the cold path used before any basin class has been indexed,
and seeds the class registry from the keys it visits.
*/
func (tree *Tree) classifyFullScan(
	sequence []byte,
	scratch *ClassificationScratch,
	keyScratch *[128]byte,
) error {
	root := tree.loadRoot()
	iterator := root.Root().Iterator()
	iterator.SeekPrefix(basinNamespaceBytes)

	for key, value, ok := iterator.Next(); ok; key, value, ok = iterator.Next() {
		if !bytes.HasPrefix(key, basinNamespaceBytes) {
			break
		}

		tree.noteBasinClass(key)
		className, basinSequence, mapped := classSequenceFromBasinKey(key)

		if !mapped || !basinMatchesSequence(basinSequence, sequence) {
			continue
		}

		if err := tree.accumulateBasinEvidence(className, basinSequence, value, scratch, keyScratch); err != nil {
			return err
		}
	}

	return nil
}

/*
accumulateClassBasins scores one attractor class by reading token-boundary
prefixes of the observed sequence under that class.
*/
func (tree *Tree) accumulateClassBasins(
	className []byte,
	sequence []byte,
	scratch *ClassificationScratch,
	keyScratch *[128]byte,
	prefixScratch *[128]byte,
) error {
	tokenStart := 0

	for index := 0; index <= len(sequence); index++ {
		if index < len(sequence) && sequence[index] != '_' {
			continue
		}

		if index == tokenStart {
			tokenStart = index + 1

			continue
		}

		path := sequence[:index]
		storageKey := tree.basinKeyScratch(className, path, keyScratch)

		if storageKey == nil {
			storageKey = basinStorageKey(className, path)
		}

		value, found := tree.getRaw(storageKey)

		if found {
			if err := tree.accumulateBasinEvidence(
				className, path, value, scratch, keyScratch,
			); err != nil {
				return err
			}
		}

		tokenStart = index + 1
	}

	_ = prefixScratch

	return nil
}

func (tree *Tree) accumulateBasinEvidence(
	className []byte,
	basinSequence []byte,
	value []byte,
	scratch *ClassificationScratch,
	keyScratch *[128]byte,
) error {
	weight := UnmarshalCognitive(value)
	parentPath := parentContextPath(basinSequence)
	parentWeight := tree.getBasinWeightStack(className, parentPath, keyScratch)
	probabilityFloor := probabilityFloorFromWeight(weight, parentWeight.Count)
	logEvidence := math.Log(math.Max(weight.Probability, probabilityFloor))

	return scratch.accumulateClassEvidence(className, logEvidence)
}

func (tree *Tree) basinKeyScratch(
	className []byte,
	sequence []byte,
	keyScratch *[128]byte,
) []byte {
	requiredLength := len(basinNamespaceBytes) + len(className) + 1 + len(sequence)

	if requiredLength > len(keyScratch) {
		return nil
	}

	storageKey := keyScratch[:requiredLength]
	offset := copy(storageKey, basinNamespaceBytes)
	offset += copy(storageKey[offset:], className)
	storageKey[offset] = '/'
	copy(storageKey[offset+1:], sequence)

	return storageKey
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

	inference, err := tree.Classify(sequence, scratch)

	if err != nil {
		return nil, 0, err
	}

	if len(inference.Winner) == 0 {
		return nil, 0, ErrNoAttractorMatch
	}

	learningRate := deriveLearningRate(tree, sequence)

	if err := tree.commitLearnDeltas(sequence, inference.Winner, learningRate); err != nil {
		return nil, 0, err
	}

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
) error {
	for index := 0; index < scratch.activeCount; index++ {
		if bytes.Equal(scratch.scores[index].ClassName, className) {
			scratch.scores[index].Value += logEvidence

			return nil
		}
	}

	if scratch.activeCount >= scratch.classLimit {
		return errors.New("dmt: classification class limit exceeded")
	}

	if len(className) > scratch.nameLimit {
		return errors.New("dmt: classification class name exceeds limit")
	}

	for len(scratch.scores) <= scratch.activeCount {
		scratch.scores = append(scratch.scores, ClassScore{})
	}

	for len(scratch.nameStorage) <= scratch.activeCount {
		scratch.nameStorage = append(scratch.nameStorage, nil)
	}

	nameBuffer := append(scratch.nameStorage[scratch.activeCount][:0], className...)
	scratch.nameStorage[scratch.activeCount] = nameBuffer

	scratch.scores[scratch.activeCount] = ClassScore{
		ClassName: nameBuffer,
		Value:     logEvidence,
	}
	scratch.activeCount++

	return nil
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

	value, found := tree.getRaw(storageKey)

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

	if !bytes.HasPrefix(sequence, basinSequence) {
		return false
	}

	if len(sequence) == len(basinSequence) {
		return true
	}

	return sequence[len(basinSequence)] == '_'
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

func (tree *Tree) commitLearnMutations(mutations []learnMutation) error {
	if tree == nil || len(mutations) == 0 {
		return nil
	}

	if tree.persist != nil {
		tree.persistMu.Lock()
		defer tree.persistMu.Unlock()

		if err := tree.persistenceError(); err != nil {
			return err
		}

		if err := tree.logLearnMutations(mutations); err != nil {
			return tree.failPersistence(err)
		}
	}

	for {
		oldRoot := tree.loadRoot()
		transaction := oldRoot.Txn()

		for _, mutation := range mutations {
			transaction.Insert(mutation.key, mutation.value)
		}

		newRoot := transaction.Commit()

		if tree.root.CompareAndSwap(oldRoot, newRoot) {
			for _, mutation := range mutations {
				tree.noteBasinClass(mutation.key)
				tree.maintainCognitiveIndexes(mutation.key, false)
			}

			return nil
		}
	}
}

/*
commitLearnDeltas applies learning as count/probability deltas under one
serialized transaction so concurrent learners cannot overwrite each other with
stale absolute replacements.
*/
func (tree *Tree) commitLearnDeltas(
	sequence []byte,
	inferredClass []byte,
	learningRate float64,
) error {
	if tree == nil || len(sequence) == 0 || len(inferredClass) == 0 {
		return nil
	}

	apply := func() []learnMutation {
		return tree.buildUnsupervisedMutations(sequence, inferredClass, learningRate)
	}

	if tree.persist == nil {
		mutations := apply()
		return tree.commitLearnMutations(mutations)
	}

	tree.persistMu.Lock()
	defer tree.persistMu.Unlock()

	if err := tree.persistenceError(); err != nil {
		return err
	}

	mutations := apply()

	if err := tree.logLearnMutations(mutations); err != nil {
		return tree.failPersistence(err)
	}

	oldRoot := tree.loadRoot()
	transaction := oldRoot.Txn()

	for _, mutation := range mutations {
		transaction.Insert(mutation.key, mutation.value)
	}

	tree.root.Store(transaction.Commit())

	for _, mutation := range mutations {
		tree.noteBasinClass(mutation.key)
		tree.maintainCognitiveIndexes(mutation.key, false)
	}

	return nil
}

/*
noteBasinClass records attractor class names so Classify can probe per-class
prefixes instead of walking every basin key on each REM replay.
*/
func (tree *Tree) noteBasinClass(key []byte) {
	if tree == nil {
		return
	}

	className, _, mapped := classSequenceFromBasinKey(key)

	if !mapped || len(className) == 0 {
		return
	}

	name := string(className)

	for {
		current := tree.basinClassNames.Load()

		if current != nil {
			for _, existing := range *current {
				if existing == name {
					return
				}
			}
		}

		next := []string{name}

		if current != nil {
			next = append(append([]string{}, (*current)...), name)
		}

		if tree.basinClassNames.CompareAndSwap(current, &next) {
			return
		}
	}
}

func (tree *Tree) basinClasses() []string {
	if tree == nil {
		return nil
	}

	current := tree.basinClassNames.Load()

	if current == nil {
		return nil
	}

	return *current
}

func (tree *Tree) logLearnMutations(mutations []learnMutation) error {
	if tree.persist == nil {
		return nil
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

	if err := tree.persist.LogInserts(entries); err != nil {
		return err
	}

	lastIndex := startIndex + uint64(len(entries))
	tree.logIndex.Store(lastIndex)

	if startIndex/tree.persist.snapCount != lastIndex/tree.persist.snapCount {
		return tree.saveSnapshotLocked(false)
	}

	return nil
}
