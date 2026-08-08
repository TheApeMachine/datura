package dmt

import (
	"bytes"
	"encoding/binary"
	"math"
)

/*
coOccurrenceWindow is how many tokens either side of a token count as its
context. Two is the narrowest window that still spans a token's neighbours on
both sides without reaching across a whole short sequence.
*/
const coOccurrenceWindow = 2

/*
fuzzyEditBudget is the edit distance within which two tokens are treated as the
same observation. One edit covers the realistic corruption of a discrete token —
a dropped or substituted character — without merging genuinely distinct names.
*/
const fuzzyEditBudget = 1

/*
observeLexicon records a sequence's tokens in the vocabulary and tallies which
tokens occur near which others.

Why:

	A trie can only answer questions about prefixes it has literally seen. The
	vocabulary and co-occurrence tallies are what let an unseen token be resolved
	to something the trie does know, so a single corrupted or novel observation
	degrades into its nearest known neighbour instead of falling off the model.
*/
func (tree *Tree) observeLexicon(sequence []byte) {
	tokens := splitUnderscoreTokens(sequence)

	if len(tokens) == 0 {
		return
	}

	for index, token := range tokens {
		if len(token) == 0 {
			continue
		}

		tree.bumpCounter(vocabularyStorageKey(token))

		low := max(0, index-coOccurrenceWindow)
		high := min(len(tokens)-1, index+coOccurrenceWindow)

		for neighbour := low; neighbour <= high; neighbour++ {
			if neighbour == index || len(tokens[neighbour]) == 0 {
				continue
			}

			tree.bumpCounter(coOccurrenceStorageKey(token, tokens[neighbour]))
		}
	}
}

/*
bumpCounter increments a plain uint64 counter stored at a key.
*/
func (tree *Tree) bumpCounter(storageKey []byte) {
	count := uint64(0)

	if value, found := tree.Get(storageKey); found && len(value) >= 8 {
		count = binary.LittleEndian.Uint64(value)
	}

	buffer := make([]byte, 8)
	binary.LittleEndian.PutUint64(buffer, count+1)
	_, _ = tree.Insert(storageKey, buffer)
}

/*
readCounter reads a plain uint64 counter.
*/
func (tree *Tree) readCounter(storageKey []byte) uint64 {
	value, found := tree.Get(storageKey)

	if !found || len(value) < 8 {
		return 0
	}

	return binary.LittleEndian.Uint64(value)
}

/*
KnowsToken reports whether a token has ever been observed.
*/
func (tree *Tree) KnowsToken(token []byte) bool {
	return tree.readCounter(vocabularyStorageKey(token)) > 0
}

/*
CoOccurrenceCount reports how often two tokens were observed within the context
window of one another.
*/
func (tree *Tree) CoOccurrenceCount(left, right []byte) uint64 {
	return tree.readCounter(coOccurrenceStorageKey(left, right))
}

/*
LexicalMatch is the resolution of an observed token onto the known vocabulary.
*/
type LexicalMatch struct {
	Original   []byte
	Mapped     []byte
	Similarity float64
	Known      bool
}

/*
ResolveToken maps an unknown token onto the closest token the model knows.

Why:

	Two resolutions are attempted in order because they answer different
	questions. Edit distance catches corruption — the same name with a character
	wrong — and is accepted immediately at near-certainty, because a one-edit
	neighbour is far more likely to be the same token than a coincidence.
	Character n-gram cosine catches family resemblance, which is the weaker claim
	and is therefore only used when nothing sits within the edit budget.

	A token the model already knows resolves to itself at similarity one without
	scanning anything.
*/
func (tree *Tree) ResolveToken(token []byte) LexicalMatch {
	if len(token) == 0 {
		return LexicalMatch{Original: token, Mapped: token, Similarity: 0}
	}

	if tree.KnowsToken(token) {
		return LexicalMatch{
			Original: token, Mapped: token, Similarity: 1, Known: true,
		}
	}

	best := LexicalMatch{Original: token, Mapped: token, Similarity: 0}
	prefix := []byte(vocabularyNamespace)

	tree.WalkPrefix(prefix, func(key, _ []byte) bool {
		known, mapped := tokenFromVocabularyKey(key)

		if !mapped || len(known) == 0 {
			return true
		}

		if abs(len(known)-len(token)) <= fuzzyEditBudget &&
			EditDistance(token, known) <= fuzzyEditBudget {
			best = LexicalMatch{
				Original:   token,
				Mapped:     append([]byte(nil), known...),
				Similarity: 0.95,
				Known:      true,
			}

			return false
		}

		similarity := NgramSimilarity(token, known)

		if similarity > best.Similarity {
			best = LexicalMatch{
				Original:   token,
				Mapped:     append([]byte(nil), known...),
				Similarity: similarity,
				Known:      true,
			}
		}

		return true
	})

	return best
}

/*
EditDistance is the Levenshtein distance between two token byte slices, computed
with a rolling row so the allocation is linear in the shorter input.
*/
func EditDistance(left, right []byte) int {
	if len(left) == 0 {
		return len(right)
	}

	if len(right) == 0 {
		return len(left)
	}

	previous := make([]int, len(right)+1)
	current := make([]int, len(right)+1)

	for index := range previous {
		previous[index] = index
	}

	for leftIndex := 1; leftIndex <= len(left); leftIndex++ {
		current[0] = leftIndex

		for rightIndex := 1; rightIndex <= len(right); rightIndex++ {
			cost := 1

			if left[leftIndex-1] == right[rightIndex-1] {
				cost = 0
			}

			current[rightIndex] = min(
				previous[rightIndex]+1,
				min(current[rightIndex-1]+1, previous[rightIndex-1]+cost),
			)
		}

		previous, current = current, previous
	}

	return previous[len(right)]
}

/*
NgramSimilarity is the cosine similarity of two tokens' character bigram
profiles, padded so the first and last characters carry position information.
*/
func NgramSimilarity(left, right []byte) float64 {
	leftGrams := characterBigrams(left)
	rightGrams := characterBigrams(right)

	if len(leftGrams) == 0 || len(rightGrams) == 0 {
		return 0
	}

	dot := 0.0
	leftMagnitude := 0.0
	rightMagnitude := 0.0

	for gram, count := range leftGrams {
		dot += float64(count) * float64(rightGrams[gram])
		leftMagnitude += float64(count) * float64(count)
	}

	for _, count := range rightGrams {
		rightMagnitude += float64(count) * float64(count)
	}

	if leftMagnitude == 0 || rightMagnitude == 0 {
		return 0
	}

	return dot / (math.Sqrt(leftMagnitude) * math.Sqrt(rightMagnitude))
}

/*
characterBigrams counts padded character bigrams. Padding with sentinels means a
token's opening and closing characters are distinguishable from the same
characters occurring mid-token.
*/
func characterBigrams(token []byte) map[string]int {
	if len(token) == 0 {
		return nil
	}

	padded := make([]byte, 0, len(token)+2)
	padded = append(padded, '^')
	padded = append(padded, token...)
	padded = append(padded, '$')

	grams := make(map[string]int, len(padded))

	for index := 0; index+2 <= len(padded); index++ {
		grams[string(padded[index:index+2])]++
	}

	return grams
}

func abs(value int) int {
	if value < 0 {
		return -value
	}

	return value
}

func tokenFromVocabularyKey(storageKey []byte) ([]byte, bool) {
	if len(storageKey) <= len(vocabularyNamespace) {
		return nil, false
	}

	if !bytes.HasPrefix(storageKey, []byte(vocabularyNamespace)) {
		return nil, false
	}

	return storageKey[len(vocabularyNamespace):], true
}
