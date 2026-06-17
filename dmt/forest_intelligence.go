package dmt

import (
	"context"
	"errors"
)

/*
GetAnalogousFallback resolves a key directly or through structural analog mapping.
*/
func (forest *Forest) GetAnalogousFallback(key []byte) ([]byte, bool) {
	tree := forest.getFastestTree()

	if tree == nil {
		return nil, false
	}

	value, found := tree.Get(key)

	if found {
		return value, true
	}

	analog, hasAnalog := tree.FindStructuralAnalog(key)

	if !hasAnalog {
		return nil, false
	}

	fallbackValue, foundValue := tree.Get(analog.ClosestKey)

	if !foundValue {
		return nil, false
	}

	forest.EvaluateCuriosityAndTriggerSync(key)

	return fallbackValue, true
}

/*
EvaluateCuriosityAndTriggerSync schedules targeted peer sync when branch entropy is high.
*/
func (forest *Forest) EvaluateCuriosityAndTriggerSync(prefix []byte) {
	tree := forest.getFastestTree()

	if tree == nil || forest.network == nil || forest.pool == nil {
		return
	}

	ambiguity := tree.MeasureBranchAmbiguity(prefix)

	if !ambiguity.Ambiguous {
		return
	}

	peersSnapshot := forest.network.peers.Load()

	if peersSnapshot.Len() == 0 {
		return
	}

	activePeers := peersSnapshot.List()
	targetPeer := activePeers[0]
	prefixCopy := append([]byte(nil), prefix...)

	forest.pool.Schedule(
		"curiosity-sync",
		func(ctx context.Context) (any, error) {
			return nil, forest.StreamTargetedPrefixSync(ctx, targetPeer.Addr(), prefixCopy)
		},
	)
}

/*
StreamTargetedPrefixSync pulls peer diff entries filtered to one sub-prefix.
*/
func (forest *Forest) StreamTargetedPrefixSync(
	ctx context.Context,
	peerAddr string,
	prefix []byte,
) error {
	if forest.network == nil {
		return errors.New("forest network is not configured")
	}

	return forest.network.StreamTargetedPrefixSync(ctx, peerAddr, prefix)
}
