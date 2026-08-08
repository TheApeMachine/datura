package dmt

import (
	"bytes"
	"math"
	"sort"
)

/*
maximumBackoffOrder is the longest suffix of context the interpolation consults.
Beyond four transitions a market context is effectively unique, so a longer order
contributes a count of one and buys nothing but work.
*/
const maximumBackoffOrder = 4

/*
smoothingMass is the pseudo-count added to every candidate at a level, so an
unobserved continuation is improbable rather than impossible.

Why:

	Without smoothing a token the model has never seen after a prefix scores
	exactly zero, and a single zero annihilates a whole log-probability sum. The
	model then reports certainty that something cannot happen on the strength of
	not having seen it yet, which is the one thing a sequence model must never do.
*/
const smoothingMass = 0.1

/*
TokenProbability is one candidate continuation and its blended probability.
*/
type TokenProbability struct {
	Token       []byte
	Probability float64
}

/*
InterpolatedProbabilities blends every backoff order of the context into one
distribution over next tokens, then folds in episodic recall.

Why:

	Reading a single prefix level makes the model brittle in exactly the situation
	it exists to handle. A context of four transitions that has never occurred
	returns nothing, even when its final two transitions are extremely familiar.
	Interpolating across orders lets a long context contribute what it knows and
	fall back on its own suffixes for the rest, so novelty degrades the estimate
	instead of erasing it.

	The order weights rise linearly rather than exponentially. An exponential
	schedule lets the longest matched order dominate outright, which reintroduces
	the brittleness the interpolation is meant to remove — one rare long match
	would silence four well-supported short ones.

	A class may be supplied to condition the distribution on one attractor basin.
	Passing nil reads the unconditioned sensory namespace.
*/
func (tree *Tree) InterpolatedProbabilities(
	contextTokens [][]byte,
	class []byte,
) []TokenProbability {
	semantic := make(map[string]float64)
	totalWeight := 0.0
	highestOrder := min(len(contextTokens), maximumBackoffOrder)

	for order := 0; order <= highestOrder; order++ {
		suffix := contextTokens[len(contextTokens)-order:]
		prefix, resolved := tree.resolveContextPath(suffix)

		if !resolved {
			continue
		}

		candidates := tree.candidateCounts(prefix, class)

		if len(candidates) == 0 {
			continue
		}

		// A longer matched order is more specific and earns more of the mass,
		// but never all of it.
		weight := float64(order + 1)
		totalWeight += weight

		observed := 0.0

		for _, count := range candidates {
			observed += count
		}

		denominator := observed + smoothingMass*float64(len(candidates))

		for token, count := range candidates {
			semantic[token] += weight * (count + smoothingMass) / denominator
		}
	}

	if totalWeight > 0 {
		for token := range semantic {
			semantic[token] /= totalWeight
		}
	}

	return tree.blendEpisodic(contextTokens, semantic)
}

/*
resolveContextPath walks a token suffix into a sensory path, substituting the
closest child when a token is not present literally.

Why:

	The trie stores exact byte paths, so a token that differs by one character
	from what was learned is a different path and the whole context is lost. A
	bounded edit substitution keeps the walk alive through that, which is the
	difference between degrading and failing.
*/
func (tree *Tree) resolveContextPath(suffix [][]byte) ([]byte, bool) {
	path := make([]byte, 0, 64)

	for _, token := range suffix {
		candidate := appendSequenceToken(nil, path, token)

		if tree.readCounter(vocabularyStorageKey(token)) > 0 {
			if state := tree.EffectiveSensoryWeight(candidate); state.Count > 0 {
				path = candidate

				continue
			}
		}

		substitute, found := tree.nearestChildToken(path, token)

		if !found {
			return nil, false
		}

		path = appendSequenceToken(nil, path, substitute)
	}

	return path, true
}

/*
nearestChildToken finds the strongest child of a prefix within the edit budget.
*/
func (tree *Tree) nearestChildToken(prefix []byte, token []byte) ([]byte, bool) {
	var best []byte

	bestCount := -1.0
	storagePrefix := sensoryStorageKey(prefix)

	tree.WalkPrefix(storagePrefix, func(key, value []byte) bool {
		sequenceKey, mapped := sequenceFromSensoryKey(key)

		if !mapped {
			return true
		}

		childToken, isChild := immediateTokenSuffix(prefix, sequenceKey)

		if !isChild || len(childToken) == 0 {
			return true
		}

		if abs(len(childToken)-len(token)) > fuzzyEditBudget ||
			EditDistance(token, childToken) > fuzzyEditBudget {
			return true
		}

		state := tree.decayValue(value)

		if float64(state.Count) > bestCount {
			bestCount = float64(state.Count)
			best = append([]byte(nil), childToken...)
		}

		return true
	})

	return best, best != nil
}

/*
candidateCounts collects the decayed counts of every immediate continuation of a
prefix, optionally restricted to one attractor basin.
*/
func (tree *Tree) candidateCounts(
	prefix []byte,
	class []byte,
) map[string]float64 {
	counts := make(map[string]float64)
	storagePrefix := sensoryStorageKey(prefix)

	tree.WalkPrefix(storagePrefix, func(key, value []byte) bool {
		sequenceKey, mapped := sequenceFromSensoryKey(key)

		if !mapped {
			return true
		}

		childToken, isChild := immediateTokenSuffix(prefix, sequenceKey)

		if !isChild || len(childToken) == 0 {
			return true
		}

		if len(class) > 0 {
			basin := tree.EffectiveWeight(basinStorageKey(class, sequenceKey))

			if basin.Count == 0 {
				return true
			}

			counts[string(childToken)] += float64(basin.Count)

			return true
		}

		counts[string(childToken)] += float64(tree.decayValue(value).Count)

		return true
	})

	return counts
}

/*
blendEpisodic folds recent verbatim experience into a distribution built from
accumulated statistics.

Why:

	Statistics need repetition before they move. A sequence seen once is almost
	invisible in the sensory counts, yet being the single most recent thing that
	happened is exactly what makes it worth predicting. The episodic buffer holds
	those raw traces, so consulting it is what gives the model one-shot recall.

	The blend weight is derived from how much of the current context the episodic
	match actually covered, rather than being a fixed share. A recalled episode
	matching the full context speaks with authority; one matching a single
	trailing token is a coincidence and is weighted like one.
*/
func (tree *Tree) blendEpisodic(
	contextTokens [][]byte,
	semantic map[string]float64,
) []TokenProbability {
	episodic, coverage := tree.EpisodicProbabilities(contextTokens)

	blended := make(map[string]float64, len(semantic)+len(episodic))
	episodicShare := coverage
	semanticShare := 1 - episodicShare

	for token, probability := range semantic {
		blended[token] += probability * semanticShare
	}

	for token, probability := range episodic {
		blended[token] += probability * episodicShare
	}

	total := 0.0

	for _, probability := range blended {
		total += probability
	}

	results := make([]TokenProbability, 0, len(blended))

	for token, probability := range blended {
		if total > 0 {
			probability /= total
		}

		results = append(results, TokenProbability{
			Token:       []byte(token),
			Probability: probability,
		})
	}

	sort.Slice(results, func(left, right int) bool {
		if results[left].Probability == results[right].Probability {
			return bytes.Compare(results[left].Token, results[right].Token) < 0
		}

		return results[left].Probability > results[right].Probability
	})

	return results
}

/*
EpisodicProbabilities searches the episodic buffer for the longest suffix of the
context that appears in a retained episode, and reports what followed it.

The returned coverage is the fraction of the context the best match spanned,
weighted by recency, and is what the caller uses to decide how much authority the
recall deserves.
*/
func (tree *Tree) EpisodicProbabilities(
	contextTokens [][]byte,
) (map[string]float64, float64) {
	probabilities := make(map[string]float64)

	if len(contextTokens) == 0 {
		return probabilities, 0
	}

	type episode struct {
		tokens [][]byte
		at     uint64
	}

	episodes := make([]episode, 0, 32)

	tree.WalkPrefix([]byte(episodicNamespace), func(key, value []byte) bool {
		timestamp, sequence, mapped := timestampFromEpisodicKey(key)

		if !mapped || len(value) == 0 {
			return true
		}

		episodes = append(episodes, episode{
			tokens: splitUnderscoreTokens(sequence),
			at:     timestamp,
		})

		return true
	})

	if len(episodes) == 0 {
		return probabilities, 0
	}

	// Newest first, so recency weighting is a function of rank rather than of an
	// absolute timestamp the caller would have to supply.
	sort.Slice(episodes, func(left, right int) bool {
		return episodes[left].at > episodes[right].at
	})

	totalWeight := 0.0
	bestCoverage := 0.0

	for rank, retained := range episodes {
		recency := math.Pow(episodicRecencyDecay, float64(rank))

		for order := len(contextTokens); order > 0; order-- {
			search := contextTokens[len(contextTokens)-order:]
			next, found := followingToken(retained.tokens, search)

			if !found {
				continue
			}

			coverage := float64(order) / float64(len(contextTokens))
			weight := recency * coverage
			probabilities[string(next)] += weight
			totalWeight += weight

			if weight > bestCoverage {
				bestCoverage = weight
			}

			break
		}
	}

	if totalWeight > 0 {
		for token := range probabilities {
			probabilities[token] /= totalWeight
		}
	}

	// Scaled, not clamped. Clamping would collapse every match at or above the
	// cap onto the same authority, so a single trailing-token coincidence would
	// speak exactly as loudly as a match spanning the whole context.
	return probabilities, bestCoverage * maximumEpisodicShare
}

/*
episodicRecencyDecay is how much authority an episode loses per newer episode
that follows it.
*/
const episodicRecencyDecay = 0.9

/*
maximumEpisodicShare is the largest share of the distribution verbatim recall may
own, earned by a match spanning the entire context at full recency. One-shot
recall must be able to raise a possibility, never to overrule the accumulated
statistics outright.
*/
const maximumEpisodicShare = 0.3

/*
followingToken finds the last occurrence of a token run inside an episode and
returns whatever followed it.
*/
func followingToken(episode [][]byte, search [][]byte) ([]byte, bool) {
	if len(search) == 0 || len(episode) <= len(search) {
		return nil, false
	}

	for start := len(episode) - len(search) - 1; start >= 0; start-- {
		matched := true

		for offset := range search {
			if !bytes.Equal(episode[start+offset], search[offset]) {
				matched = false

				break
			}
		}

		if matched {
			return episode[start+len(search)], true
		}
	}

	return nil, false
}
