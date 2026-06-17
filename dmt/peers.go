package dmt

import (
	"sort"
	"sync/atomic"
)

type peerEntry struct {
	addr string
	node *peer
}

/*
Peers manages cluster node targets using a lock-free sorted flat slice.
*/
type Peers struct {
	entries []peerEntry
}

func (peers *Peers) load() *Peers {
	if peers == nil {
		return &Peers{entries: make([]peerEntry, 0)}
	}

	return peers
}

func (peers *Peers) List() []*peer {
	current := peers.load()
	peerList := make([]*peer, len(current.entries))

	for index, entry := range current.entries {
		peerList[index] = entry.node
	}

	return peerList
}

func (peers *Peers) Has(addr string) bool {
	current := peers.load()
	entryCount := len(current.entries)

	entryIndex := sort.Search(entryCount, func(index int) bool {
		return current.entries[index].addr >= addr
	})

	return entryIndex < entryCount && current.entries[entryIndex].addr == addr
}

func (peers *Peers) With(addr string, peerEntryNode *peer) *Peers {
	current := peers.load()
	currentEntries := current.entries
	entryCount := len(currentEntries)

	entryIndex := sort.Search(entryCount, func(index int) bool {
		return currentEntries[index].addr >= addr
	})

	if entryIndex < entryCount && currentEntries[entryIndex].addr == addr {
		nextEntries := make([]peerEntry, entryCount)
		copy(nextEntries, currentEntries)
		nextEntries[entryIndex].node = peerEntryNode

		return &Peers{entries: nextEntries}
	}

	nextEntries := make([]peerEntry, entryCount+1)
	copy(nextEntries[:entryIndex], currentEntries[:entryIndex])
	nextEntries[entryIndex] = peerEntry{addr: addr, node: peerEntryNode}
	copy(nextEntries[entryIndex+1:], currentEntries[entryIndex:])

	return &Peers{entries: nextEntries}
}

func (peers *Peers) Without(addr string) *Peers {
	current := peers.load()
	currentEntries := current.entries
	entryCount := len(currentEntries)

	entryIndex := sort.Search(entryCount, func(index int) bool {
		return currentEntries[index].addr >= addr
	})

	if entryIndex >= entryCount || currentEntries[entryIndex].addr != addr {
		return current
	}

	nextEntries := make([]peerEntry, entryCount-1)
	copy(nextEntries[:entryIndex], currentEntries[:entryIndex])
	copy(nextEntries[entryIndex:], currentEntries[entryIndex+1:])

	return &Peers{entries: nextEntries}
}

func (peers *Peers) Len() int {
	return len(peers.load().entries)
}

/*
peerRegistry publishes peer snapshot updates through atomic pointers.
*/
type peerRegistry struct {
	snapshot atomic.Pointer[Peers]
}

func newPeerRegistry() *peerRegistry {
	registry := &peerRegistry{}
	registry.snapshot.Store(&Peers{entries: make([]peerEntry, 0)})

	return registry
}

func (registry *peerRegistry) Load() *Peers {
	return registry.snapshot.Load()
}

func (registry *peerRegistry) Store(peers *Peers) {
	registry.snapshot.Store(peers)
}

func (registry *peerRegistry) Upsert(addr string, peerEntry *peer) {
	for {
		current := registry.Load()
		next := current.With(addr, peerEntry)

		if registry.snapshot.CompareAndSwap(current, next) {
			return
		}
	}
}

func (registry *peerRegistry) Remove(addr string) {
	for {
		current := registry.Load()
		next := current.Without(addr)

		if registry.snapshot.CompareAndSwap(current, next) {
			return
		}
	}
}
