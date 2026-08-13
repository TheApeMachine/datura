package dmt

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
func (inference *CognitiveInference) InterpolatedProbabilities(
	contextTokens [][]byte,
	class []byte,
) []TokenProbability {
	return inference.interpolatedProbabilities(
		contextTokens, class, inference.episodes(),
	)
}

func (inference *CognitiveInference) interpolatedProbabilities(
	contextTokens [][]byte,
	class []byte,
	episodes []episode,
) []TokenProbability {
	semantic := make(map[string]float64)
	totalWeight := 0.0
	highestOrder := min(len(contextTokens), maximumBackoffOrder)

	for order := 0; order <= highestOrder; order++ {
		suffix := contextTokens[len(contextTokens)-order:]
		prefix, resolved := inference.resolveContextPath(suffix)

		if !resolved {
			continue
		}

		candidates := inference.candidateCounts(prefix, class)

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

	return inference.blendEpisodic(contextTokens, semantic, episodes)
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
func (inference *CognitiveInference) resolveContextPath(
	suffix [][]byte,
) ([]byte, bool) {
	path := make([]byte, 0, 64)

	for _, token := range suffix {
		candidate := appendSequenceToken(nil, path, token)

		if inference.tree.readCounter(vocabularyStorageKey(token)) > 0 {
			if state := inference.tree.EffectiveSensoryWeight(candidate); state.Count > 0 {
				path = candidate

				continue
			}

			return nil, false
		}

		substitute, found := inference.nearestChildToken(path, token)

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
func (inference *CognitiveInference) nearestChildToken(
	prefix []byte,
	token []byte,
) ([]byte, bool) {
	var best []byte

	bestCount := -1.0
	storagePrefix := sensoryStorageKey(prefix)

	inference.tree.WalkPrefix(storagePrefix, func(key, value []byte) bool {
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

		state := inference.tree.decayValue(value)

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
func (inference *CognitiveInference) candidateCounts(
	prefix []byte,
	class []byte,
) map[string]float64 {
	counts := make(map[string]float64)
	storagePrefix := sensoryStorageKey(prefix)

	if len(class) > 0 {
		storagePrefix = basinStorageKey(class, prefix)
	}

	inference.tree.WalkPrefix(storagePrefix, func(key, value []byte) bool {
		sequenceKey, mapped := sequenceFromSensoryKey(key)

		if len(class) > 0 {
			_, sequenceKey, mapped = classSequenceFromBasinKey(key)
		}

		if !mapped {
			return true
		}

		childToken, isChild := immediateTokenSuffix(prefix, sequenceKey)

		if !isChild || len(childToken) == 0 {
			return true
		}

		if len(class) > 0 {
			basin := inference.tree.decayValue(value)

			if basin.Count == 0 {
				return true
			}

			counts[string(childToken)] += float64(basin.Count)

			return true
		}

		counts[string(childToken)] += float64(inference.tree.decayValue(value).Count)

		return true
	})

	return counts
}
