package dmt

import (
	"math"
	"sort"
)

/*
minimumSymbolEvidence is how much accumulated weight a candidate needs before it
can be called discriminative. Below it a perfect one-to-one association is a
single coincidence.
*/
const minimumSymbolEvidence = 2.0

/*
DiscriminativeSymbol is a token path that predicts one class disproportionately.
*/
type DiscriminativeSymbol struct {
	Symbol []byte  `json:"symbol"`
	Class  []byte  `json:"class"`
	Score  float64 `json:"score"`
	Weight float64 `json:"weight"`
	Purity float64 `json:"purity"`
}

/*
ExtractDiscriminativeSymbols finds the token paths that most sharply identify a
class.

Why:

	The classifier reports which class won, never which fragment of the sequence
	did the work. A path that occurs across every class is structure the model has
	learned but that carries no decision value, and it is indistinguishable from a
	decisive one by count alone.

	Three factors multiply, and each rules out a different way of being wrong:

	  - Purity, P(class | symbol), is the discriminative claim itself. A path
	    split evenly across classes scores near the uniform share and drops out.
	  - log(1 + weight) is confidence in that claim. It grows with evidence but
	    with diminishing returns, so a path seen a thousand times cannot outrank
	    everything on volume alone.
	  - The square root of path length rewards specificity. A longer path is a
	    stronger commitment about context, but the reward is sublinear because
	    length also makes a path rarer.
*/
func (tree *Tree) ExtractDiscriminativeSymbols(limit int) []DiscriminativeSymbol {
	if limit <= 0 {
		return nil
	}

	classes := tree.KnownClasses()

	if len(classes) == 0 {
		return nil
	}

	// weightsBySymbol[symbol][class] is decayed basin evidence.
	weightsBySymbol := make(map[string]map[string]float64)

	tree.WalkPrefix(basinNamespaceBytes, func(key, value []byte) bool {
		class, sequence, mapped := classSequenceFromBasinKey(key)

		if !mapped || len(sequence) == 0 {
			return true
		}

		state := tree.decayValue(value)

		if state.Count == 0 {
			return true
		}

		byClass, found := weightsBySymbol[string(sequence)]

		if !found {
			byClass = make(map[string]float64, len(classes))
			weightsBySymbol[string(sequence)] = byClass
		}

		byClass[string(class)] += float64(state.Count)

		return true
	})

	uniform := 1.0 / float64(len(classes))
	symbols := make([]DiscriminativeSymbol, 0, len(weightsBySymbol))

	for sequence, byClass := range weightsBySymbol {
		total := 0.0

		for _, weight := range byClass {
			total += weight
		}

		if total < minimumSymbolEvidence {
			continue
		}

		depth := float64(len(splitUnderscoreTokens([]byte(sequence))))

		if depth <= 0 {
			continue
		}

		for class, weight := range byClass {
			purity := weight / total

			// A path no more associated with this class than an even split would
			// predict is not evidence about it.
			if purity <= uniform {
				continue
			}

			symbols = append(symbols, DiscriminativeSymbol{
				Symbol: []byte(sequence),
				Class:  []byte(class),
				Score:  purity * math.Log(1+weight) * math.Sqrt(depth),
				Weight: weight,
				Purity: purity,
			})
		}
	}

	sort.Slice(symbols, func(left, right int) bool {
		return symbols[left].Score > symbols[right].Score
	})

	if len(symbols) > limit {
		symbols = symbols[:limit]
	}

	return symbols
}
