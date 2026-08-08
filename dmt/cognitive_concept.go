package dmt

import (
	"bytes"
	"fmt"
	"math"
	"sort"
)

/*
TokenContribution is one token's signed evidence for the winning class over the
runner-up, in bits.
*/
type TokenContribution struct {
	Token []byte  `json:"token"`
	Bits  float64 `json:"bits"`
}

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
InterpolatedSurprisal scores each token of a sequence against the interpolated
distribution its own prefix implies.

This differs from GetSurprisal, which reads a single stored path. Scoring
through the interpolation means an unseen continuation of a familiar prefix is
merely surprising rather than infinitely so, which is what makes the value usable
as a control signal.
*/
func (tree *Tree) InterpolatedSurprisal(sequence []byte) []SurprisalItem {
	tokens := splitUnderscoreTokens(sequence)
	items := make([]SurprisalItem, 0, len(tokens))

	for index, token := range tokens {
		low := max(0, index-maximumBackoffOrder)
		context := tokens[low:index]
		probability := tree.probabilityOf(context, token, nil)

		items = append(items, SurprisalItem{
			Token:     append([]byte(nil), token...),
			Surprisal: -math.Log2(probability),
		})
	}

	return items
}

/*
probabilityOf reads one token's probability out of the interpolated
distribution, flooring it so an unobserved continuation stays finite.
*/
func (tree *Tree) probabilityOf(
	context [][]byte,
	token []byte,
	class []byte,
) float64 {
	for _, candidate := range tree.InterpolatedProbabilities(context, class) {
		if bytes.Equal(candidate.Token, token) {
			if candidate.Probability > 0 {
				return candidate.Probability
			}

			break
		}
	}

	return unobservedProbabilityFloor
}

/*
unobservedProbabilityFloor is the probability assigned to a continuation the
interpolation never proposed, bounding surprisal at roughly ten bits.
*/
const unobservedProbabilityFloor = 0.001

/*
ClassifyInterpolated scores every known class over a sequence using the
interpolated distribution, and reports each token's contribution.

Why:

	The basin classifier answers which stored attractor a sequence matches. This
	answers a different question — how well each class's own transition
	statistics generate the sequence — and it can therefore score a sequence no
	basin has ever recorded. Both are useful, and they disagree in exactly the
	informative case: a novel sequence.
*/
func (tree *Tree) ClassifyInterpolated(
	sequence []byte,
) (ClassificationResult, map[string][]TokenContribution) {
	classes := tree.KnownClasses()

	if len(classes) == 0 {
		return ClassificationResult{}, nil
	}

	tokens := splitUnderscoreTokens(sequence)
	logProbabilities := make([]float64, len(classes))
	contributions := make(map[string][]TokenContribution, len(classes))
	observations := float64(tree.CognitiveStep())

	if observations <= 0 {
		observations = 1
	}

	for classIndex, class := range classes {
		prior := float64(tree.classObservationCount(class))

		if prior <= 0 {
			prior = smoothingMass
		}

		logProbability := math.Log(prior / observations)
		perToken := make([]TokenContribution, 0, len(tokens))

		for index, token := range tokens {
			low := max(0, index-maximumBackoffOrder)
			probability := tree.probabilityOf(tokens[low:index], token, class)
			bits := math.Log(probability)
			logProbability += bits

			perToken = append(perToken, TokenContribution{
				Token: append([]byte(nil), token...),
				Bits:  bits,
			})
		}

		logProbabilities[classIndex] = logProbability
		contributions[string(class)] = perToken
	}

	scores := make([]ClassScore, len(classes))

	for index, class := range classes {
		scores[index] = ClassScore{
			ClassName: class,
			Value:     logProbabilities[index],
		}
	}

	normalizeLogEvidence(scores)
	sortClassScoresDescending(scores)

	return ClassificationResult{
		Scores:  scores,
		Winner:  scores[0].ClassName,
		Highest: scores[0].Value,
	}, contributions
}

/*
ContrastiveTokenContributions reports, per token, how much more evidence it gave
the winning class than the runner-up.

Why:

	A verdict with a score is not an explanation. Knowing that one transition
	carried the whole decision, and that the rest of the sequence was neutral
	between the two candidates, is what makes a classification auditable rather
	than merely reported.
*/
func (tree *Tree) ContrastiveTokenContributions(
	sequence []byte,
) []TokenContribution {
	result, contributions := tree.ClassifyInterpolated(sequence)

	if len(result.Scores) < 2 {
		return nil
	}

	winner := contributions[string(result.Scores[0].ClassName)]
	runnerUp := contributions[string(result.Scores[1].ClassName)]
	differences := make([]TokenContribution, 0, len(winner))

	for index, contribution := range winner {
		if index >= len(runnerUp) {
			break
		}

		differences = append(differences, TokenContribution{
			Token: contribution.Token,
			Bits:  (contribution.Bits - runnerUp[index].Bits) / math.Ln2,
		})
	}

	return differences
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
