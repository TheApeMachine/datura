package dmt

import (
	"sync/atomic"
)

/*
Peers is an immutable view of connected network peers.
Writers publish a new snapshot pointer; readers load it without locking.
*/
type Peers struct {
	byAddr map[string]*peer
}

func (peers *Peers) load() *Peers {
	if peers == nil {
		return &Peers{byAddr: make(map[string]*peer)}
	}

	return peers
}

func (peers *Peers) List() []*peer {
	current := peers.load()
	peerList := make([]*peer, 0, len(current.byAddr))

	for _, peerEntry := range current.byAddr {
		peerList = append(peerList, peerEntry)
	}

	return peerList
}

func (peers *Peers) Has(addr string) bool {
	_, exists := peers.load().byAddr[addr]

	return exists
}

func (peers *Peers) With(addr string, peerEntry *peer) *Peers {
	current := peers.load()
	nextByAddr := make(map[string]*peer, len(current.byAddr)+1)

	for key, value := range current.byAddr {
		nextByAddr[key] = value
	}

	nextByAddr[addr] = peerEntry

	return &Peers{byAddr: nextByAddr}
}

func (peers *Peers) Without(addr string) *Peers {
	current := peers.load()

	if _, exists := current.byAddr[addr]; !exists {
		return current
	}

	nextByAddr := make(map[string]*peer, len(current.byAddr)-1)

	for key, value := range current.byAddr {
		if key == addr {
			continue
		}

		nextByAddr[key] = value
	}

	return &Peers{byAddr: nextByAddr}
}

func (peers *Peers) Len() int {
	return len(peers.load().byAddr)
}

/*
peerRegistry publishes peer map updates through atomic snapshot pointers.
*/
type peerRegistry struct {
	snapshot atomic.Pointer[Peers]
}

func newPeerRegistry() *peerRegistry {
	registry := &peerRegistry{}
	registry.snapshot.Store(&Peers{byAddr: make(map[string]*peer)})

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
