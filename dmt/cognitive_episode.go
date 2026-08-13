package dmt

import (
	"bytes"
	"math"
	"sort"
)

/*
episode is one parsed episodic observation reused throughout an analysis.
*/
type episode struct {
	tokens [][]byte
	at     uint64
}

/*
episodes reads and parses the retained episodic namespace once in recency order.
*/
func (inference *CognitiveInference) episodes() []episode {
	root := inference.tree.loadRoot()
	inference.cacheMu.RLock()

	if inference.episodeRoot == root {
		episodes := inference.episodeCache
		inference.cacheMu.RUnlock()

		return episodes
	}

	inference.cacheMu.RUnlock()
	episodes := make([]episode, 0, 32)
	iterator := root.Root().Iterator()
	iterator.SeekPrefix([]byte(episodicNamespace))

	for key, value, ok := iterator.Next(); ok; key, value, ok = iterator.Next() {
		timestamp, sequence, mapped := timestampFromEpisodicKey(key)

		if !mapped || len(value) == 0 {
			continue
		}

		episodes = append(episodes, episode{
			tokens: splitUnderscoreTokens(sequence),
			at:     timestamp,
		})

	}

	sort.Slice(episodes, func(left, right int) bool {
		return episodes[left].at > episodes[right].at
	})
	inference.cacheMu.Lock()
	inference.episodeRoot = root
	inference.episodeCache = episodes
	inference.cacheMu.Unlock()

	return episodes
}

/*
blendEpisodic folds recent verbatim experience into a statistical distribution.
*/
func (inference *CognitiveInference) blendEpisodic(
	contextTokens [][]byte,
	semantic map[string]float64,
	episodes []episode,
) []TokenProbability {
	episodic, coverage := inference.episodicProbabilities(contextTokens, episodes)
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
EpisodicProbabilities searches retained episodes for the longest context suffix
and reports the continuation distribution and its recency-weighted coverage.
*/
func (inference *CognitiveInference) EpisodicProbabilities(
	contextTokens [][]byte,
) (map[string]float64, float64) {
	return inference.episodicProbabilities(contextTokens, inference.episodes())
}

func (inference *CognitiveInference) episodicProbabilities(
	contextTokens [][]byte,
	episodes []episode,
) (map[string]float64, float64) {
	probabilities := make(map[string]float64)

	if len(contextTokens) == 0 || len(episodes) == 0 {
		return probabilities, 0
	}

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
			bestCoverage = max(bestCoverage, weight)

			break
		}
	}

	if totalWeight > 0 {
		for token := range probabilities {
			probabilities[token] /= totalWeight
		}
	}

	return probabilities, bestCoverage * maximumEpisodicShare
}

/*
episodicRecencyDecay is how much authority an episode loses per newer episode.
*/
const episodicRecencyDecay = 0.9

/*
maximumEpisodicShare is the most authority verbatim recall may earn. It can
raise a once-seen possibility without overruling accumulated statistics.
*/
const maximumEpisodicShare = 0.3

/*
followingToken finds the last occurrence of a token run inside an episode.
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
