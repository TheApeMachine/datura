package dmt

import (
	"bytes"
	"fmt"
	"sort"
)

/*
ExperienceOutcome is what one unsupervised observation resolved to.
*/
type ExperienceOutcome struct {
	Class        []byte  `json:"class"`
	Surprisal    float64 `json:"surprisal"`
	LearningRate float64 `json:"learningRate"`
	NewConcept   bool    `json:"newConcept"`
}

/*
ExperienceSequence learns from an observation without being told its class,
spawning a new concept when no existing class explains it.

Why:

	A classifier bounded by a predefined taxonomy can only ever sort the world
	into boxes someone else drew. The market does not consult that list. When a
	recurring sequence matches nothing the model knows, the honest response is to
	name it and start accumulating evidence about it, not to force it into the
	least-bad existing basin and corrupt that basin's statistics.

	The spawn threshold is derived rather than fixed: a winner must beat what an
	even split across the known classes would have produced. With two classes that
	is 50%, with ten it is 10%, and neither number has to be chosen.
*/
func (tree *Tree) ExperienceSequence(
	sequence []byte,
	scratch *ClassificationScratch,
) (ExperienceOutcome, error) {
	if len(sequence) == 0 {
		return ExperienceOutcome{}, ErrEmptySequence
	}

	if tree == nil || scratch == nil {
		return ExperienceOutcome{}, ErrNoAttractorMatch
	}

	surprisalItems := tree.InterpolatedSurprisal(sequence)
	averageSurprisal := 0.0

	for _, item := range surprisalItems {
		averageSurprisal += item.Surprisal
	}

	if len(surprisalItems) > 0 {
		averageSurprisal /= float64(len(surprisalItems))
	}

	classes := tree.KnownClasses()
	inference := tree.Classify(sequence, scratch)
	spawn := len(classes) == 0

	if !spawn && len(inference.Winner) == 0 {
		spawn = true
	}

	if !spawn {
		uniform := 1.0 / float64(len(classes))

		if inference.Highest < uniform {
			spawn = true
		}
	}

	class := inference.Winner

	if spawn {
		class = tree.spawnConcept()
	}

	learningRate := deriveLearningRate(tree, sequence)
	mutations := tree.buildUnsupervisedMutations(sequence, class, learningRate)
	tree.commitLearnMutations(mutations)
	tree.TrainSensorySequence(sequence)
	tree.registerClass(class)

	return ExperienceOutcome{
		Class:        class,
		Surprisal:    averageSurprisal,
		LearningRate: learningRate,
		NewConcept:   spawn,
	}, nil
}

/*
TeachSequence learns a sequence under a class the caller supplies.

UnsupervisedLearn and ExperienceSequence both infer the class from what the
model already believes, which cannot bootstrap a taxonomy and cannot correct one.
Supervised training is the entry point for a caller that knows the answer.
*/
func (tree *Tree) TeachSequence(sequence []byte, class []byte) error {
	if len(sequence) == 0 {
		return ErrEmptySequence
	}

	if tree == nil || len(class) == 0 {
		return ErrNoAttractorMatch
	}

	learningRate := deriveLearningRate(tree, sequence)
	tree.commitLearnMutations(
		tree.buildUnsupervisedMutations(sequence, class, learningRate),
	)
	tree.TrainSensorySequence(sequence)
	tree.registerClass(class)

	return nil
}

/*
spawnConcept allocates the next unnamed concept identity.
*/
func (tree *Tree) spawnConcept() []byte {
	next := tree.conceptCounter.Add(1)

	return fmt.Appendf(nil, "concept_%d", next)
}

/*
registerClass records that a class exists and has been observed once more, which
is what supplies the prior in interpolated classification.
*/
func (tree *Tree) registerClass(class []byte) {
	if len(class) == 0 {
		return
	}

	tree.bumpCounter(conceptStorageKey(class))
}

/*
classObservationCount reports how many observations a class has accumulated.
*/
func (tree *Tree) classObservationCount(class []byte) uint64 {
	return tree.readCounter(conceptStorageKey(class))
}

/*
KnownClasses lists every class the model has recorded, whether it was taught or
spawned. Classes are discovered from the basin namespace as well as the concept
register, so a tree trained before concepts existed still reports its classes.
*/
func (tree *Tree) KnownClasses() [][]byte {
	seen := make(map[string]struct{})
	classes := make([][]byte, 0, 8)

	tree.WalkPrefix([]byte(conceptNamespace), func(key, _ []byte) bool {
		if len(key) <= len(conceptNamespace) {
			return true
		}

		class := key[len(conceptNamespace):]

		if _, found := seen[string(class)]; !found {
			seen[string(class)] = struct{}{}
			classes = append(classes, append([]byte(nil), class...))
		}

		return true
	})

	tree.WalkPrefix(basinNamespaceBytes, func(key, _ []byte) bool {
		class, _, mapped := classSequenceFromBasinKey(key)

		if !mapped {
			return true
		}

		if _, found := seen[string(class)]; !found {
			seen[string(class)] = struct{}{}
			classes = append(classes, append([]byte(nil), class...))
		}

		return true
	})

	sort.Slice(classes, func(left, right int) bool {
		return bytes.Compare(classes[left], classes[right]) < 0
	})

	return classes
}

func conceptStorageKey(class []byte) []byte {
	storageKey := make([]byte, len(conceptNamespace)+len(class))
	copy(storageKey, conceptNamespace)
	copy(storageKey[len(conceptNamespace):], class)

	return storageKey
}
