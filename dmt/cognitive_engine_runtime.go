package dmt

import (
	"bytes"
	"math"
	"strings"

	"github.com/theapemachine/datura"
)

/*
CognitiveEngine performs opt-in cognition against a Tree.
It observes storage state and writes ordinary tree mutations; storage inserts do
not call it.
*/
type CognitiveEngine struct {
	tree *Tree
}

/*
NewCognitiveEngine binds cognitive work to a tree without putting that work on
the tree storage lifecycle.
*/
func NewCognitiveEngine(tree *Tree) *CognitiveEngine {
	return &CognitiveEngine{tree: tree}
}

/*
WithCognition is a compatibility adapter for existing callers.
*/
func (tree *Tree) WithCognition(artifact *datura.Artifact) *datura.Artifact {
	return NewCognitiveEngine(tree).Stamp(artifact)
}

/*
Stamp scores the artifact against tree memory, learns the observation, and
stamps cognitive fields onto the payload.
*/
func (engine *CognitiveEngine) Stamp(artifact *datura.Artifact) *datura.Artifact {
	if engine == nil || engine.tree == nil || artifact == nil {
		return artifact
	}

	tree := engine.tree
	sequence := engine.sequence(artifact)
	parentSequence := sequence

	if underscore := bytes.LastIndex(sequence, []byte{'_'}); underscore >= 0 {
		parentSequence = sequence[:underscore]
	}

	parentWeight := tree.GetContextWeight(parentSequence)
	classification, contrastEvidence := engine.classification(sequence)
	ambiguity := tree.MeasureBranchAmbiguity(sequence)
	lookaheadScore, lookaheadPaths := engine.lookahead(sequence)

	engine.train(sequence)
	engine.writeArtifact(
		artifact,
		sequence,
		parentSequence,
		parentWeight,
		classification,
		contrastEvidence,
		ambiguity,
		lookaheadScore,
		lookaheadPaths,
	)

	return artifact
}

func (engine *CognitiveEngine) sequence(artifact *datura.Artifact) []byte {
	return []byte(strings.Join([]string{
		datura.Peek[string](artifact, "scope"),
		datura.Peek[string](artifact, "origin"),
		datura.Peek[string](artifact, "role"),
	}, "_"))
}

func (engine *CognitiveEngine) classification(
	sequence []byte,
) (ClassificationResult, float64) {
	var classifyScratch ClassificationScratch

	classification := engine.tree.Classify(sequence, &classifyScratch)
	contrastEvidence := 0.0

	if len(classification.Scores) < 2 {
		return classification, contrastEvidence
	}

	evidence := engine.tree.ComputeBasinContrastiveEvidence(
		classification.Scores[0].ClassName,
		classification.Scores[1].ClassName,
		sequence,
	)

	return classification, evidence.Divergence
}

func (engine *CognitiveEngine) lookahead(sequence []byte) (float64, int) {
	var lookaheadBuffer [32]LookaheadPrediction

	lookahead := engine.tree.PredictNextSensoryTokens(sequence, lookaheadBuffer[:0])
	score := 0.0

	for _, prediction := range lookahead {
		score += prediction.Probability
	}

	return score, len(lookahead)
}

func (engine *CognitiveEngine) train(sequence []byte) {
	tokenStart := 0

	for index := 0; index <= len(sequence); index++ {
		if index < len(sequence) && sequence[index] != '_' {
			continue
		}

		if index == tokenStart {
			tokenStart = index + 1

			continue
		}

		engine.trainPath(sequence[:index])
		tokenStart = index + 1
	}

	engine.tree.TrainSensorySequence(sequence)
}

func (engine *CognitiveEngine) trainPath(currentPath []byte) {
	parentPath := parentContextPath(currentPath)
	current := engine.tree.GetContextWeight(currentPath)
	parent := engine.tree.GetContextWeight(parentPath)
	nextCount := current.Count + 1
	probability := 1.0

	if len(parentPath) > 0 {
		denominator := float64(parent.Count + 1)

		if denominator <= 0 {
			denominator = float64(nextCount)
		}

		probability = float64(nextCount) / denominator
	}

	engine.tree.InsertContextWeight(currentPath, PackedWeight{
		Count:       nextCount,
		Probability: probability,
	})
}

func (engine *CognitiveEngine) writeArtifact(
	artifact *datura.Artifact,
	sequence []byte,
	parentSequence []byte,
	parentWeight PackedWeight,
	classification ClassificationResult,
	contrastEvidence float64,
	ambiguity AmbiguityState,
	lookaheadScore float64,
	lookaheadPaths int,
) {
	surprise := engine.surprise(sequence)
	surpriseThreshold := -math.Log2(1.0 / float64(parentWeight.Count+1))

	artifact.Poke(surprise, "cognition", "surprise", "value")
	artifact.Poke(surpriseThreshold, "cognition", "surprise", "threshold")
	artifact.Poke(ambiguity.EntropyBits, "cognition", "ambiguity", "bits")
	artifact.Poke(ambiguity.Threshold, "cognition", "ambiguity", "threshold")
	artifact.Poke(ambiguity.Ambiguous, "cognition", "ambiguity", "ambiguous")
	artifact.Poke(classification.Highest, "cognition", "classification", "highest")
	artifact.Poke(contrastEvidence, "cognition", "classification", "divergence")
	artifact.Poke(string(classification.Winner), "cognition", "classification", "winner")
	artifact.Poke(lookaheadScore, "cognition", "lookahead", "score")
	artifact.Poke(lookaheadPaths, "cognition", "lookahead", "paths")
	artifact.Poke(string(sequence), "cognition", "sequence", "value")
	artifact.Poke(string(parentSequence), "cognition", "sequence", "regime", "prefix")
	artifact.Poke(parentWeight.Count, "cognition", "sequence", "regime", "cohort")
}

func (engine *CognitiveEngine) surprise(sequence []byte) float64 {
	total := 0.0

	for _, item := range engine.tree.GetSurprisal(sequence) {
		total += item.Surprisal
	}

	return total
}
