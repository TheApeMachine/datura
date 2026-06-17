package dmt

import "hash/fnv"

/*
hashNodeID maps a node identifier to a stable scalar for lock-free election state.
*/
func hashNodeID(nodeID string) uint64 {
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(nodeID))

	return hasher.Sum64()
}
