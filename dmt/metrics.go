/*
package dmt implements metrics tracking for the radix tree system.
This includes performance metrics, operational counters, and network statistics
that help monitor and optimize the distributed tree's behavior.
*/
package dmt

import (
	"math"
	"sync/atomic"
	"time"
)

/*
Metrics tracks performance and operational metrics for the radix tree.
*/
type Metrics struct {
	insertCount   atomic.Uint64
	lookupCount   atomic.Uint64
	syncCount     atomic.Uint64
	conflictCount atomic.Uint64
	votesReceived atomic.Uint64
	termNumber    atomic.Uint64
	lastVoter     atomic.Pointer[string]
	insertLatency  *Latency
	lookupLatency  *Latency
	syncLatency    *Latency
	networkLatency *Latency
	bytesTransmitted atomic.Uint64
	bytesReceived    atomic.Uint64
	peerCount        atomic.Int32
	isLeader         atomic.Bool
	nodeRole         atomic.Pointer[string]
	nodeWeightBits   atomic.Uint64
	lastSyncUnixNano atomic.Int64
}

/*
NewMetrics creates a new metrics tracker with initialized latency trackers.
*/
func NewMetrics() *Metrics {
	metrics := &Metrics{
		insertLatency:  NewLatency(100),
		lookupLatency:  NewLatency(100),
		syncLatency:    NewLatency(100),
		networkLatency: NewLatency(100),
	}

	emptyRole := ""
	metrics.nodeRole.Store(&emptyRole)

	return metrics
}

func storeString(target *atomic.Pointer[string], value string) {
	copied := value
	target.Store(&copied)
}

func loadString(source *atomic.Pointer[string]) string {
	value := source.Load()

	if value == nil {
		return ""
	}

	return *value
}

func storeFloat64Bits(target *atomic.Uint64, value float64) {
	target.Store(math.Float64bits(value))
}

func loadFloat64Bits(source *atomic.Uint64) float64 {
	return math.Float64frombits(source.Load())
}

/*
RecordInsert records metrics for an insert operation.
*/
func (metrics *Metrics) RecordInsert(duration time.Duration, bytes int) {
	metrics.insertCount.Add(1)
	metrics.insertLatency.Record(duration)
	metrics.bytesTransmitted.Add(uint64(bytes))
}

/*
RecordLookup records metrics for a lookup operation.
*/
func (metrics *Metrics) RecordLookup(duration time.Duration) {
	metrics.lookupCount.Add(1)
	metrics.lookupLatency.Record(duration)
}

/*
RecordSync records metrics for a sync operation.
*/
func (metrics *Metrics) RecordSync(duration time.Duration, bytes int) {
	metrics.syncCount.Add(1)
	metrics.syncLatency.Record(duration)
	metrics.bytesReceived.Add(uint64(bytes))
	metrics.lastSyncUnixNano.Store(time.Now().UnixNano())
}

/*
RecordConflict records a detected conflict during operations.
*/
func (metrics *Metrics) RecordConflict() {
	metrics.conflictCount.Add(1)
}

/*
UpdatePeerCount updates the current peer count.
*/
func (metrics *Metrics) UpdatePeerCount(count int32) {
	metrics.peerCount.Store(count)
}

/*
SetNodeRole updates the node's role and weight in the network.
*/
func (metrics *Metrics) SetNodeRole(role string, weight float64) {
	storeString(&metrics.nodeRole, role)
	storeFloat64Bits(&metrics.nodeWeightBits, weight)
}

/*
SetLeader updates the node's leader status.
*/
func (metrics *Metrics) SetLeader(isLeader bool) {
	metrics.isLeader.Store(isLeader)
}

/*
RecordVote records a vote received during election.
*/
func (metrics *Metrics) RecordVote(voter string) {
	metrics.votesReceived.Add(1)
	storeString(&metrics.lastVoter, voter)
}

/*
GetMetrics returns a snapshot of current metrics.
*/
func (metrics *Metrics) GetMetrics() map[string]interface{} {
	lastSyncNano := metrics.lastSyncUnixNano.Load()
	lastSyncTime := time.Time{}

	if lastSyncNano > 0 {
		lastSyncTime = time.Unix(0, lastSyncNano)
	}

	return map[string]interface{}{
		"operations": map[string]uint64{
			"insert":   metrics.insertCount.Load(),
			"lookup":   metrics.lookupCount.Load(),
			"sync":     metrics.syncCount.Load(),
			"conflict": metrics.conflictCount.Load(),
		},
		"election": map[string]interface{}{
			"votes_received": metrics.votesReceived.Load(),
			"term_number":    metrics.termNumber.Load(),
			"last_voter":     loadString(&metrics.lastVoter),
		},
		"latencies": map[string]float64{
			"insert":  float64(metrics.insertLatency.Average()) / float64(time.Millisecond),
			"lookup":  float64(metrics.lookupLatency.Average()) / float64(time.Millisecond),
			"sync":    float64(metrics.syncLatency.Average()) / float64(time.Millisecond),
			"network": float64(metrics.networkLatency.Average()) / float64(time.Millisecond),
		},
		"network": map[string]interface{}{
			"bytes_tx":   metrics.bytesTransmitted.Load(),
			"bytes_rx":   metrics.bytesReceived.Load(),
			"peer_count": metrics.peerCount.Load(),
		},
		"node": map[string]interface{}{
			"is_leader":      metrics.isLeader.Load(),
			"role":           loadString(&metrics.nodeRole),
			"weight":         loadFloat64Bits(&metrics.nodeWeightBits),
			"last_sync_time": lastSyncTime,
		},
	}
}
