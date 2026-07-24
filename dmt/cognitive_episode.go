package dmt

import "bytes"

/*
CommitToEpisodicBuffer stores one observation in the timestamp-ordered episodic
namespace so REM can replay the exact interval requested by the caller.
*/
func (tree *Tree) CommitToEpisodicBuffer(
	timestamp uint64,
	sequence []byte,
) (*Tree, bool, error) {
	return tree.Insert(episodicStorageKey(timestamp, sequence), []byte{1})
}

/*
ExecuteREMSleepConsolidation replays and consumes one episodic interval. Removing
only successfully replayed entries makes the namespace a buffer rather than an
unbounded history and prevents later REM passes from training on the same event.
*/
func (tree *Tree) ExecuteREMSleepConsolidation(
	startTimestamp uint64,
	endTimestamp uint64,
) error {
	replayedKeys := make([][]byte, 0)
	preservedSequences := make([][]byte, 0)

	tree.WalkLowerBound(
		episodicStorageKey(startTimestamp, nil),
		func(storageKey, value []byte) bool {
			if !bytes.HasPrefix(storageKey, []byte(episodicNamespace)) {
				return false
			}

			timestamp, sequence, mapped := timestampFromEpisodicKey(storageKey)

			if !mapped || len(value) == 0 {
				return true
			}

			if timestamp > endTimestamp {
				return false
			}

			tree.TrainSensorySequenceAt(timestamp, sequence)
			preservedSequences = append(
				preservedSequences,
				sensoryPrefixPaths(sequence)...,
			)

			var classifyScratch ClassificationScratch
			_ = tree.optimizeWeightsInline(sequence, &classifyScratch)
			replayedKeys = append(replayedKeys, append([]byte(nil), storageKey...))

			return true
		},
	)

	namespaceEntries := countNamespaceEntries(tree, []byte(sensoryNamespace))

	tree.applyDecayConsolidation(
		[]byte(sensoryNamespace),
		0,
		endTimestamp,
		namespaceEntries,
		preservedSequences...,
	)

	return tree.consumeEpisodes(replayedKeys)
}

/*
consumeEpisodes deletes replayed keys through Tree.Delete so in-memory and
durable trees use the same ordered mutation contract.
*/
func (tree *Tree) consumeEpisodes(replayedKeys [][]byte) error {
	for _, storageKey := range replayedKeys {
		if _, _, err := tree.Delete(storageKey); err != nil {
			return err
		}
	}

	return nil
}
