package dmt

import (
	"encoding/binary"
	"math"
)

/*
stampedWeightSize is the packed layout extended with the step the weight was
last written at. The unstamped 16-byte layout remains readable, so a tree
persisted before lazy decay existed still loads; its entries simply carry step
zero and are treated as maximally stale on first touch.
*/
const stampedWeightSize = 24

/*
cognitiveDecayFactor is the per-step retention applied to a weight between the
step it was written at and the step it is read at.

Why:

	Consolidation already decays the sensory namespace, but it runs on a period.
	Between two consolidations every count is frozen, so a pattern that stopped
	occurring keeps its full weight and keeps winning until the next pass sweeps
	it. Reading through a decay makes staleness continuous instead of a staircase,
	and costs one pow per read rather than a walk over the namespace.
*/
const cognitiveDecayFactor = 0.995

/*
MarshalStampedWeight encodes count, probability, and the write step.
*/
func MarshalStampedWeight(count uint64, probability float64, step uint64) []byte {
	buffer := make([]byte, stampedWeightSize)

	binary.LittleEndian.PutUint64(buffer[0:8], count)
	binary.LittleEndian.PutUint64(buffer[8:16], math.Float64bits(probability))
	binary.LittleEndian.PutUint64(buffer[16:24], step)

	return buffer
}

/*
UnmarshalStampedStep reads the write step from a packed buffer, reporting false
for the legacy unstamped layout.
*/
func UnmarshalStampedStep(buffer []byte) (uint64, bool) {
	if len(buffer) < stampedWeightSize {
		return 0, false
	}

	return binary.LittleEndian.Uint64(buffer[16:24]), true
}

/*
CognitiveStep reports the tree's monotonic cognitive clock. The clock advances
once per trained sequence, so it measures experience rather than wall time and a
quiet market does not age the model.
*/
func (tree *Tree) CognitiveStep() uint64 {
	if tree == nil {
		return 0
	}

	return tree.cognitiveStep.Load()
}

/*
AdvanceCognitiveStep moves the clock forward one observation and returns the new
value.
*/
func (tree *Tree) AdvanceCognitiveStep() uint64 {
	if tree == nil {
		return 0
	}

	return tree.cognitiveStep.Add(1)
}

/*
decayMultiplier is the retention a weight written at writeStep has accumulated
by the current step.
*/
func (tree *Tree) decayMultiplier(writeStep uint64) float64 {
	current := tree.CognitiveStep()

	if writeStep >= current {
		return 1
	}

	return math.Pow(cognitiveDecayFactor, float64(current-writeStep))
}

/*
EffectiveWeight reads a stored weight through the decay accumulated since it was
written. A caller that wants the raw stored counts uses GetSensoryWeight; a
caller ranking or comparing patterns wants this.
*/
func (tree *Tree) EffectiveWeight(storageKey []byte) CognitiveState {
	value, found := tree.Get(storageKey)

	if !found {
		return CognitiveState{}
	}

	state := UnmarshalCognitive(value)
	writeStep, stamped := UnmarshalStampedStep(value)

	if !stamped {
		// A legacy entry has no provenance, so it is aged from the clock's
		// origin rather than being credited as freshly written.
		writeStep = 0
	}

	multiplier := tree.decayMultiplier(writeStep)
	state.Probability *= multiplier
	state.Count = uint64(float64(state.Count) * multiplier)

	return state
}

/*
EffectiveSensoryWeight is EffectiveWeight over the sensory namespace.
*/
func (tree *Tree) EffectiveSensoryWeight(sequence []byte) CognitiveState {
	return tree.EffectiveWeight(sensoryStorageKey(sequence))
}

/*
decayValue applies the decay a raw stored buffer has accumulated, for callers
already holding the value from an iteration rather than a point lookup.
*/
func (tree *Tree) decayValue(value []byte) CognitiveState {
	state := UnmarshalCognitive(value)
	writeStep, _ := UnmarshalStampedStep(value)
	multiplier := tree.decayMultiplier(writeStep)
	state.Probability *= multiplier
	state.Count = uint64(float64(state.Count) * multiplier)

	return state
}

/*
marshalStamped encodes a cognitive state at the tree's current step, so every
write records when it happened and later reads can age it.
*/
func (tree *Tree) marshalStamped(state CognitiveState) []byte {
	return MarshalStampedWeight(state.Count, state.Probability, tree.CognitiveStep())
}
