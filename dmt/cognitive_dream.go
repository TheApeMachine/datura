package dmt

import (
	"bytes"
)

/*
repetitionPenalty is the factor applied to a token already produced in the recent
window of a generated sequence.

Why:

	A model sampling from its own most probable continuation will happily emit a
	two-token cycle forever, because each step is individually the likeliest one.
	The penalty is what makes the walk explore rather than orbit, and it decays out
	of the window so a genuinely repeating pattern can still be expressed.
*/
const repetitionPenalty = 0.5

/*
repetitionWindow is how many recently generated tokens the penalty considers.
*/
const repetitionWindow = 3

/*
dreamNoveltyConfidence is how strongly a generated sequence must classify back to
the basin that produced it before it is worth consolidating.

A dream that does not classify as its own class is not a crisp memory of that
class; training on it would blur the basin rather than sharpen it.
*/
const dreamNoveltyConfidence = 0.85

/*
GenerateSequence walks the interpolated distribution to produce a plausible
continuation, sampled at a temperature.

Temperature zero takes the strongest continuation at every step. Higher values
flatten the distribution toward exploration. The generator is deterministic for a
given selector, which is supplied by the caller so this package holds no
randomness of its own.
*/
func (tree *Tree) GenerateSequence(
	context []byte,
	class []byte,
	temperature float64,
	maximumTokens int,
	selector func(candidates []CandidateToken, temperature float64) []byte,
) []byte {
	if maximumTokens <= 0 || selector == nil {
		return nil
	}

	tokens := splitUnderscoreTokens(context)
	generated := make([][]byte, 0, maximumTokens)

	for step := 0; step < maximumTokens; step++ {
		low := max(0, len(tokens)-maximumBackoffOrder)
		distribution := tree.InterpolatedProbabilities(tokens[low:], class)

		if len(distribution) == 0 {
			break
		}

		candidates := make([]CandidateToken, 0, len(distribution))

		for _, entry := range distribution {
			score := entry.Probability

			if recentlyGenerated(generated, entry.Token) {
				score *= repetitionPenalty
			}

			candidates = append(candidates, CandidateToken{
				Token: entry.Token,
				Score: score,
			})
		}

		next := selector(candidates, temperature)

		if len(next) == 0 {
			break
		}

		tokens = append(tokens, next)
		generated = append(generated, next)
	}

	if len(generated) == 0 {
		return nil
	}

	return joinUnderscoreTokens(generated)
}

/*
recentlyGenerated reports whether a token appears in the trailing repetition
window.
*/
func recentlyGenerated(generated [][]byte, token []byte) bool {
	low := max(0, len(generated)-repetitionWindow)

	for _, previous := range generated[low:] {
		if bytes.Equal(previous, token) {
			return true
		}
	}

	return false
}

/*
DreamOutcome records one consolidated generated sequence.
*/
type DreamOutcome struct {
	Sequence   []byte  `json:"sequence"`
	Class      []byte  `json:"class"`
	Confidence float64 `json:"confidence"`
}

/*
ExecuteDreamConsolidation generates candidate sequences from each known basin and
trains the ones that are both novel and crisp.

Why:

	Replaying what was observed can only reinforce what the model already holds.
	Generating from a basin and keeping what comes back sharply classified is how
	it fills in the region around what it has seen — the continuations that are
	implied by its statistics but never happened to occur.

	Two gates keep this from being a feedback loop that amplifies its own noise.
	Novelty means the sequence is not already a stored path, so nothing is
	double-counted. Confidence means the sequence classifies back to the basin
	that produced it, so only unambiguous inventions are kept. A generated
	sequence that fails either gate is discarded, not weakened and stored.
*/
func (tree *Tree) ExecuteDreamConsolidation(
	temperature float64,
	maximumTokens int,
	scratch *ClassificationScratch,
	selector func(candidates []CandidateToken, temperature float64) []byte,
) []DreamOutcome {
	if tree == nil || scratch == nil || selector == nil {
		return nil
	}

	classes := tree.KnownClasses()

	if len(classes) == 0 {
		return nil
	}

	outcomes := make([]DreamOutcome, 0, len(classes))

	for _, class := range classes {
		dreamed := tree.GenerateSequence(
			nil, class, temperature, maximumTokens, selector,
		)

		if len(dreamed) == 0 {
			continue
		}

		if tree.EffectiveSensoryWeight(dreamed).Count > 0 {
			// Already known; consolidating it would only re-count experience.
			continue
		}

		inference := tree.Classify(dreamed, scratch)

		if !bytes.Equal(inference.Winner, class) ||
			inference.Highest < dreamNoveltyConfidence {
			continue
		}

		learningRate := deriveLearningRate(tree, dreamed)
		tree.commitLearnMutations(
			tree.buildUnsupervisedMutations(dreamed, class, learningRate),
		)
		tree.TrainSensorySequence(dreamed)

		outcomes = append(outcomes, DreamOutcome{
			Sequence:   dreamed,
			Class:      append([]byte(nil), class...),
			Confidence: inference.Highest,
		})
	}

	return outcomes
}

/*
joinUnderscoreTokens is the inverse of splitUnderscoreTokens.
*/
func joinUnderscoreTokens(tokens [][]byte) []byte {
	if len(tokens) == 0 {
		return nil
	}

	size := len(tokens) - 1

	for _, token := range tokens {
		size += len(token)
	}

	joined := make([]byte, 0, size)

	for index, token := range tokens {
		if index > 0 {
			joined = append(joined, '_')
		}

		joined = append(joined, token...)
	}

	return joined
}
