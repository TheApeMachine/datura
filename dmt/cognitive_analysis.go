package dmt

import (
	"bytes"
	"math"
	"sync"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
)

/*
unobservedProbabilityFloor is the probability assigned to a continuation the
interpolation never proposed, bounding surprisal at roughly ten bits.
*/
const unobservedProbabilityFloor = 0.001

/*
CognitiveInference owns DMT's read-only cognitive derivations over a Tree.
*/
type CognitiveInference struct {
	tree          *Tree
	cacheMu       sync.RWMutex
	episodeRoot   *iradix.Tree[[]byte]
	episodeCache  []episode
	analysisRoot  *iradix.Tree[[]byte]
	analysisCache map[string]InterpolatedAnalysis
}

/*
TokenContribution is one token's signed evidence for the winning class over the
runner-up, in bits.
*/
type TokenContribution struct {
	Token []byte  `json:"token"`
	Bits  float64 `json:"bits"`
}

/*
InterpolatedAnalysis is DMT's complete generative explanation of one sequence.
Its slices are immutable views owned by CognitiveInference.
*/
type InterpolatedAnalysis struct {
	Classification   ClassificationResult
	Surprisal        []SurprisalItem
	AverageSurprisal float64
	Contributions    []TokenContribution
}

/*
AnalyzeInterpolated computes surprisal and contrast from one episodic snapshot.
*/
func (inference *CognitiveInference) AnalyzeInterpolated(
	sequence []byte,
) InterpolatedAnalysis {
	root := inference.tree.loadRoot()
	inference.cacheMu.RLock()
	analysis, found := inference.analysisCache[string(sequence)]
	cacheCurrent := inference.analysisRoot == root
	inference.cacheMu.RUnlock()

	if cacheCurrent && found {
		return analysis
	}

	analysis = inference.analyzeInterpolated(sequence)

	if inference.tree.loadRoot() != root {
		return analysis
	}

	inference.cacheMu.Lock()

	if inference.analysisRoot != root {
		inference.analysisRoot = root
		inference.analysisCache = make(map[string]InterpolatedAnalysis)
	}

	inference.analysisCache[string(sequence)] = analysis
	inference.cacheMu.Unlock()

	return analysis
}

func (inference *CognitiveInference) analyzeInterpolated(
	sequence []byte,
) InterpolatedAnalysis {
	episodes := inference.episodes()
	classification, byClass := inference.classifyInterpolated(sequence, episodes)
	surprisal := inference.interpolatedSurprisal(sequence, episodes)
	averageSurprisal := 0.0

	for _, item := range surprisal {
		averageSurprisal += item.Surprisal
	}

	if len(surprisal) > 0 {
		averageSurprisal /= float64(len(surprisal))
	}

	return InterpolatedAnalysis{
		Classification:   classification,
		Surprisal:        surprisal,
		AverageSurprisal: averageSurprisal,
		Contributions:    inference.contrastiveContributions(classification, byClass),
	}
}

/*
InterpolatedSurprisal scores each token through the model's backoff distribution.
*/
func (inference *CognitiveInference) InterpolatedSurprisal(
	sequence []byte,
) []SurprisalItem {
	return inference.interpolatedSurprisal(sequence, inference.episodes())
}

func (inference *CognitiveInference) interpolatedSurprisal(
	sequence []byte,
	episodes []episode,
) []SurprisalItem {
	tokens := splitUnderscoreTokens(sequence)
	items := make([]SurprisalItem, 0, len(tokens))

	for index, token := range tokens {
		low := max(0, index-maximumBackoffOrder)
		probability := inference.probabilityOf(
			tokens[low:index], token, nil, episodes,
		)

		items = append(items, SurprisalItem{
			Token:     append([]byte(nil), token...),
			Surprisal: -math.Log2(probability),
		})
	}

	return items
}

func (inference *CognitiveInference) probabilityOf(
	context [][]byte,
	token []byte,
	class []byte,
	episodes []episode,
) float64 {
	for _, candidate := range inference.interpolatedProbabilities(
		context, class, episodes,
	) {
		if bytes.Equal(candidate.Token, token) && candidate.Probability > 0 {
			return candidate.Probability
		}
	}

	return unobservedProbabilityFloor
}

/*
ClassifyInterpolated scores every known class with interpolated token evidence.
*/
func (inference *CognitiveInference) ClassifyInterpolated(
	sequence []byte,
) (ClassificationResult, map[string][]TokenContribution) {
	return inference.classifyInterpolated(sequence, inference.episodes())
}

func (inference *CognitiveInference) classifyInterpolated(
	sequence []byte,
	episodes []episode,
) (ClassificationResult, map[string][]TokenContribution) {
	classes := inference.tree.KnownClasses()

	if len(classes) == 0 {
		return ClassificationResult{}, nil
	}

	tokens := splitUnderscoreTokens(sequence)
	logProbabilities := make([]float64, len(classes))
	contributions := make(map[string][]TokenContribution, len(classes))
	observations := max(float64(inference.tree.CognitiveStep()), 1)

	for classIndex, class := range classes {
		prior := float64(inference.tree.classObservationCount(class))

		if prior <= 0 {
			prior = smoothingMass
		}

		logProbability := math.Log(prior / observations)
		perToken := make([]TokenContribution, 0, len(tokens))

		for index, token := range tokens {
			low := max(0, index-maximumBackoffOrder)
			probability := inference.probabilityOf(
				tokens[low:index], token, class, episodes,
			)
			bits := math.Log(probability)
			logProbability += bits
			perToken = append(perToken, TokenContribution{
				Token: append([]byte(nil), token...), Bits: bits,
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
		Scores: scores, Winner: scores[0].ClassName, Highest: scores[0].Value,
	}, contributions
}

/*
ContrastiveTokenContributions reports each token's evidence for the interpolated
winner over its runner-up.
*/
func (inference *CognitiveInference) ContrastiveTokenContributions(
	sequence []byte,
) []TokenContribution {
	result, contributions := inference.classifyInterpolated(
		sequence, inference.episodes(),
	)

	return inference.contrastiveContributions(result, contributions)
}

func (inference *CognitiveInference) contrastiveContributions(
	result ClassificationResult,
	contributions map[string][]TokenContribution,
) []TokenContribution {
	if len(result.Scores) < 2 {
		return nil
	}

	winner := contributions[string(result.Scores[0].ClassName)]
	runnerUp := contributions[string(result.Scores[1].ClassName)]
	differences := make([]TokenContribution, 0, min(len(winner), len(runnerUp)))

	for index := range min(len(winner), len(runnerUp)) {
		differences = append(differences, TokenContribution{
			Token: winner[index].Token,
			Bits:  (winner[index].Bits - runnerUp[index].Bits) / math.Ln2,
		})
	}

	return differences
}
