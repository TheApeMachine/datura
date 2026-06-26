/*
package dmt implements a wrapper around an immutable radix tree data structure.
A radix tree (also known as a radix trie or compact prefix tree) is a space-optimized
tree structure that is particularly efficient for string or byte slice keys. It compresses
common prefixes to save space and enables fast lookups, insertions, and prefix-based searches.
*/
package dmt

import (
	"bytes"
	"iter"
	"math"
	"strings"
	"sync/atomic"
	"time"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

/*
Tree wraps an immutable radix tree implementation from hashicorp/go-immutable-radix.
It stores byte slices as both keys and values, providing efficient prefix-based operations.
Readers load the root pointer atomically; writers publish new roots with compare-and-swap.
*/
type Tree struct {
	state        *batch
	root         atomic.Pointer[iradix.Tree[[]byte]]
	persist      *PersistentStore
	term         atomic.Uint64
	logIndex     atomic.Uint64
	opCount      atomic.Uint64
	opTotalNanos atomic.Int64
}

func (tree *Tree) loadRoot() *iradix.Tree[[]byte] {
	if tree == nil {
		return iradix.New[[]byte]()
	}

	root := tree.root.Load()

	if root != nil {
		return root
	}

	return iradix.New[[]byte]()
}

const treeOpSampleMask = uint64(63)

func (tree *Tree) beginOp() (started time.Time, track bool) {
	if tree == nil {
		return time.Time{}, false
	}

	if tree.opCount.Load()&treeOpSampleMask != 0 {
		return time.Time{}, false
	}

	return time.Now(), true
}

/*
endOp increments the operation count and tracks the time taken.
*/
func (tree *Tree) endOp(started time.Time, track bool) {
	if tree == nil {
		return
	}

	tree.opCount.Add(1)

	if !track {
		return
	}

	tree.opTotalNanos.Add(time.Since(started).Nanoseconds())
}

/*
NewTree creates and returns a new empty Tree instance.
The underlying radix tree is initialized with no entries.
*/
func NewTree(persistDir string) *Tree {
	tree := &Tree{
		state: newBatch("dmt/tree"),
	}

	emptyRoot := iradix.New[[]byte]()
	tree.root.Store(emptyRoot)

	if persistDir != "" {
		tree.persist = guardValue(tree.state, func() (*PersistentStore, error) {
			return NewPersistentStore(persistDir)
		})

		entries := guardValue(tree.state, tree.persist.Replay)
		root := tree.loadRoot()

		for _, entry := range entries {
			if entry.Op == opDelete {
				root, _, _ = root.Delete(entry.Key)
				continue
			}

			root, _, _ = root.Insert(entry.Key, entry.Value)
		}

		tree.root.Store(root)

		term, index := tree.persist.GetLastState()
		tree.term.Store(term)
		tree.logIndex.Store(index)
	}

	return tree
}

/*
Seek performs a prefix-based search in the tree, and returns anything
matching the longest common prefix.
*/
func (tree *Tree) Seek(key []byte) iter.Seq[*datura.Artifact] {
	started, track := tree.beginOp()
	root := tree.loadRoot()

	it := root.Root().Iterator()
	it.SeekPrefix(key)

	return iter.Seq[*datura.Artifact](func(yield func(*datura.Artifact) bool) {
		for foundKey, value, ok := it.Next(); ok; foundKey, value, ok = it.Next() {
			if !bytes.HasPrefix(foundKey, key) {
				break
			}

			if len(value) == 0 {
				continue
			}

			inbound := datura.Acquire("dmt/tree", datura.APPJSON)

			if _, err := inbound.Unpack(value); err != nil {
				errnie.Error(errnie.Err(
					errnie.Validation, "failed to unpack artifact", err,
				))
				continue
			}

			if !yield(inbound) {
				tree.endOp(started, track)
				return
			}
		}

		tree.endOp(started, track)
	})
}

/*
WalkPrefix visits every key/value whose key begins with prefix, in
lexicographical (and therefore chronological, given Artifact.Prefix keys) order.
The walk stops early if fn returns false. This is the history read: write
observations keyed by Artifact.Prefix, then walk the scope prefix to replay them.
*/
func (tree *Tree) WalkPrefix(prefix []byte, fn func(key, value []byte) bool) {
	started, track := tree.beginOp()
	root := tree.loadRoot()

	it := root.Root().Iterator()
	it.SeekPrefix(prefix)

	for key, value, ok := it.Next(); ok; key, value, ok = it.Next() {
		if !fn(key, value) {
			tree.endOp(started, track)

			return
		}
	}

	tree.endOp(started, track)
}

/*
Insert adds or updates a key-value pair in the tree.
Due to the immutable nature of the tree, this operation creates a new version
of the tree rather than modifying the existing one.
Returns the updated tree and a boolean indicating if the tree was modified.
*/
func (tree *Tree) Insert(key []byte, value []byte) (*Tree, bool) {
	started, track := tree.beginOp()

	for {
		oldRoot := tree.loadRoot()
		newRoot, _, _ := oldRoot.Insert(key, value)

		if newRoot == oldRoot {
			tree.endOp(started, track)

			return tree, false
		}

		if tree.root.CompareAndSwap(oldRoot, newRoot) {
			index := tree.logIndex.Add(1)

			if tree.persist != nil {
				guardStep(tree.state, func() error {
					return tree.persist.LogInsert(
						key,
						value,
						tree.term.Load(),
						index,
					)
				})
			}

			tree.endOp(started, track)

			return tree, true
		}
	}
}

/*
InsertArtifact adds or updates a datura.Artifact in the tree.
Due to the immutable nature of the tree, this operation creates a new version
of the tree rather than modifying the existing one.
Returns the updated tree and a boolean indicating if the tree was modified.
*/
func (tree *Tree) InsertArtifact(
	prefix []byte,
	artifact *datura.Artifact,
) (*Tree, bool) {
	if tree == nil || artifact == nil || len(prefix) == 0 {
		return tree, false
	}

	wire := artifact.Pack()

	if len(wire) == 0 {
		return tree, false
	}

	return tree.Insert(prefix, wire)
}

/*
WithCognition scores the artifact against tree memory, learns the observation,
and stamps cognitive fields onto the payload.
*/
func (tree *Tree) WithCognition(artifact *datura.Artifact) *datura.Artifact {
	if tree == nil || artifact == nil {
		return artifact
	}

	sequence := []byte(strings.Join([]string{
		datura.Peek[string](artifact, "scope"),
		datura.Peek[string](artifact, "origin"),
		datura.Peek[string](artifact, "role"),
	}, "_"))

	var surprise float64

	for _, item := range tree.GetSurprisal(sequence) {
		surprise += item.Surprisal
	}

	parentSequence := sequence

	if underscore := bytes.LastIndex(sequence, []byte{'_'}); underscore >= 0 {
		parentSequence = sequence[:underscore]
	}

	parentWeight := tree.GetContextWeight(parentSequence)
	surpriseThreshold := -math.Log2(1.0 / float64(parentWeight.Count+1))

	ambiguity := tree.MeasureBranchAmbiguity(sequence)

	var classifyScratch ClassificationScratch

	classification := tree.Classify(sequence, &classifyScratch)

	contrastEvidence := 0.0

	if len(classification.Scores) >= 2 {
		evidence := tree.ComputeBasinContrastiveEvidence(
			classification.Scores[0].ClassName,
			classification.Scores[1].ClassName,
			sequence,
		)
		contrastEvidence = evidence.Divergence
	}

	var lookaheadBuffer [32]LookaheadPrediction

	lookahead := tree.PredictNextSensoryTokens(sequence, lookaheadBuffer[:0])
	lookaheadScore := 0.0

	for _, prediction := range lookahead {
		lookaheadScore += prediction.Probability
	}

	tokenStart := 0

	for index := 0; index <= len(sequence); index++ {
		if index < len(sequence) && sequence[index] != '_' {
			continue
		}

		if index == tokenStart {
			tokenStart = index + 1

			continue
		}

		currentPath := sequence[:index]
		parentPath := parentContextPath(currentPath)
		current := tree.GetContextWeight(currentPath)
		parent := tree.GetContextWeight(parentPath)
		nextCount := current.Count + 1
		probability := 1.0

		if len(parentPath) > 0 {
			denominator := float64(parent.Count + 1)

			if denominator <= 0 {
				denominator = float64(nextCount)
			}

			probability = float64(nextCount) / denominator
		}

		tree.InsertContextWeight(currentPath, PackedWeight{
			Count:       nextCount,
			Probability: probability,
		})

		tokenStart = index + 1
	}

	tree.TrainSensorySequence(sequence)

	artifact.Poke(surprise, "cognition", "surprise", "value")
	artifact.Poke(surpriseThreshold, "cognition", "surprise", "threshold")
	artifact.Poke(ambiguity.EntropyBits, "cognition", "ambiguity", "bits")
	artifact.Poke(ambiguity.Threshold, "cognition", "ambiguity", "threshold")
	artifact.Poke(ambiguity.Ambiguous, "cognition", "ambiguity", "ambiguous")
	artifact.Poke(classification.Highest, "cognition", "classification", "highest")
	artifact.Poke(contrastEvidence, "cognition", "classification", "divergence")
	artifact.Poke(string(classification.Winner), "cognition", "classification", "winner")
	artifact.Poke(lookaheadScore, "cognition", "lookahead", "score")
	artifact.Poke(len(lookahead), "cognition", "lookahead", "paths")
	artifact.Poke(string(sequence), "cognition", "sequence", "value")
	artifact.Poke(string(parentSequence), "cognition", "sequence", "regime", "prefix")
	artifact.Poke(parentWeight.Count, "cognition", "sequence", "regime", "cohort")

	return artifact
}

/*
Get retrieves the value associated with the given key.
Returns the value and true if the key exists, or nil and false if it doesn't.
*/
func (tree *Tree) Get(key []byte) ([]byte, bool) {
	started, track := tree.beginOp()
	value, ok := tree.loadRoot().Get(key)
	tree.endOp(started, track)

	return value, ok
}

/*
AVG returns the average performance of the tree in nanoseconds.
*/
func (tree *Tree) AVG() int64 {
	if tree == nil {
		return 0
	}

	count := tree.opCount.Load()

	if count == 0 {
		return 0
	}

	return tree.opTotalNanos.Load() * int64(treeOpSampleMask+1) / int64(count)
}

/*
Close closes the tree and persists any remaining data.
*/
func (tree *Tree) Close() error {
	if tree == nil {
		return nil
	}

	if tree.persist != nil {
		guardStep(tree.state, tree.persist.Close)
	}

	return tree.state.Err()
}

func (tree *Tree) UpdateTerm(term uint64) {
	if tree == nil {
		return
	}

	tree.term.Store(term)

	if tree.persist != nil {
		guardStep(tree.state, func() error {
			return tree.persist.LogTerm(term)
		})
	}
}

func (tree *Tree) GetLogState() (term, index uint64) {
	if tree == nil {
		return 0, 0
	}

	return tree.term.Load(), tree.logIndex.Load()
}
