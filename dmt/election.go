/*
package dmt implements leader election functionality for the distributed radix tree.
It uses a Raft-like consensus algorithm to maintain a consistent leader across the
network and handle leader failures gracefully.
*/
package dmt

import (
	"context"
	"math/rand"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
)

/*
NodeState represents the current state of a node in the election process.
*/
type NodeState int

const (
	Follower NodeState = iota
	Candidate
	Leader
)

/*
ElectionConfig holds configuration for leader election.
*/
type ElectionConfig struct {
	ElectionTimeout   time.Duration
	HeartbeatInterval time.Duration
	QuorumSize        int
}

/*
Election manages the leader election process.
*/
type Election struct {
	config         ElectionConfig
	node           *NetworkNode
	role           atomic.Uint32
	term           atomic.Uint64
	votedFor       atomic.Uint64
	lastLogTerm    atomic.Uint64
	lastLogIndex   atomic.Uint64
	votesReceived  atomic.Uint32
	votesNeeded    atomic.Uint32
	electionTimer  *time.Timer
	heartbeatTimer *time.Timer
	voteRing       *structure.MPMCRing[uint64]
	closed         atomic.Bool
}

/*
NewElection creates a new election manager.
*/
func NewElection(config ElectionConfig, node *NetworkNode) *Election {
	election := &Election{
		config:         config,
		node:           node,
		heartbeatTimer: time.NewTimer(0),
	}

	election.role.Store(uint32(Follower))
	election.heartbeatTimer.Stop()

	voteRing, err := structure.NewMPMCRing[uint64](node.ctx, 128)

	if errnie.Error(err) == nil {
		election.voteRing = voteRing
	}

	election.resetElectionTimer()

	election.scheduleLoop("run-loop", func(jobCtx context.Context) (any, error) {
		election.tick(jobCtx)

		return nil, nil
	})

	return election
}

func (election *Election) tick(jobCtx context.Context) {
	for !election.closed.Load() {
		select {
		case <-jobCtx.Done():
			return

		case <-election.electionTimer.C:
			election.startElection()

		case <-election.heartbeatTimer.C:
			if election.getState() == Leader {
				election.sendHeartbeats()
			}
		}
	}
}

func (election *Election) startElection() {
	election.role.Store(uint32(Candidate))
	currentTerm := election.term.Add(1)
	election.votedFor.Store(hashNodeID(election.node.config.NodeID))

	election.node.metrics.SetLeader(false)

	peers := election.node.peers.Load().List()
	votesNeeded := max(election.config.QuorumSize, (len(peers)/2)+1)
	election.votesNeeded.Store(uint32(votesNeeded))
	election.votesReceived.Store(1)

	for _, peerEntry := range peers {
		peer := peerEntry
		election.schedule("request-vote-"+peer.addr, func(ctx context.Context) (any, error) {
			future, release := peer.client.RequestVote(election.node.ctx, func(params RadixRPC_requestVote_Params) error {
				params.SetTerm(currentTerm)
				params.SetCandidateId(election.node.config.NodeID)
				params.SetLastLogIndex(election.getLastLogIndex())
				params.SetLastLogTerm(election.getLastLogTerm())

				return nil
			})
			defer release()

			result := errnie.Does(future.Struct)

			if result.Err() != nil {
				return nil, result.Err()
			}

			if result.Value().VoteGranted() {
				election.publishVote(peer.nodeIDHash)
				election.drainVotes()
			}

			return nil, nil
		})
	}

	election.resetElectionTimer()
	election.tryPromoteToLeader()
}

func (election *Election) tryPromoteToLeader() {
	if election.votesReceived.Load() < election.votesNeeded.Load() {
		return
	}

	if election.role.CompareAndSwap(uint32(Candidate), uint32(Leader)) {
		election.becomeLeader()
	}
}

func (election *Election) becomeLeader() {
	election.role.Store(uint32(Leader))
	election.node.metrics.SetLeader(true)
	election.heartbeatTimer = time.NewTimer(election.config.HeartbeatInterval)
}

func (election *Election) sendHeartbeats() {
	peers := election.node.peers.Load().List()

	for _, peerEntry := range peers {
		peer := peerEntry
		election.schedule("heartbeat-"+peer.addr, func(ctx context.Context) (any, error) {
			future, release := peer.client.Heartbeat(election.node.ctx, func(params RadixRPC_heartbeat_Params) error {
				params.SetTerm(election.term.Load())
				params.SetLeaderId(election.node.config.NodeID)

				return nil
			})
			defer release()

			result := errnie.Does(future.Struct)

			if result.Err() != nil {
				return nil, result.Err()
			}

			heartbeat := result.Value()

			if heartbeat.Term() > election.term.Load() {
				election.stepDown(heartbeat.Term())
			}

			return nil, nil
		})
	}

	election.heartbeatTimer.Reset(election.config.HeartbeatInterval)
}

func (election *Election) stepDown(newTerm uint64) {
	election.stepDownLocked(newTerm)
}

func (election *Election) stepDownLocked(newTerm uint64) {
	election.role.Store(uint32(Follower))
	election.term.Store(newTerm)
	election.votedFor.Store(0)
	election.node.metrics.SetLeader(false)
	election.resetElectionTimer()
}

func (election *Election) resetElectionTimer() {
	if election.electionTimer != nil {
		election.electionTimer.Stop()
	}

	jitter := time.Duration(rand.Int63n(int64(election.config.ElectionTimeout)))
	timeout := election.config.ElectionTimeout + jitter
	election.electionTimer = time.NewTimer(timeout)
}

func (election *Election) getState() NodeState {
	return NodeState(election.role.Load())
}

func (election *Election) publishVote(voterID uint64) {
	if election.voteRing == nil || voterID == 0 {
		return
	}

	for !election.voteRing.Push(voterID) {
		if election.closed.Load() {
			return
		}

		runtime.Gosched()
	}
}

func (election *Election) drainVotes() {
	if election.voteRing == nil {
		return
	}

	for {
		voterID := election.voteRing.Pop()

		if voterID == 0 {
			return
		}

		if election.getState() != Candidate {
			return
		}

		received := election.votesReceived.Add(1)

		if received >= election.votesNeeded.Load() {
			election.tryPromoteToLeader()

			return
		}
	}
}

func (election *Election) handleVoteRequest(
	term uint64,
	candidateId string,
	lastLogIndex uint64,
	lastLogTerm uint64,
) bool {
	currentTerm := election.term.Load()

	if term > currentTerm {
		election.stepDownLocked(term)
		currentTerm = term
	}

	candidateHash := hashNodeID(candidateId)
	votedFor := election.votedFor.Load()

	if term < currentTerm || (votedFor != 0 && votedFor != candidateHash) {
		return false
	}

	logOK := lastLogTerm > election.lastLogTerm.Load() ||
		(lastLogTerm == election.lastLogTerm.Load() && lastLogIndex >= election.lastLogIndex.Load())

	if !logOK {
		return false
	}

	election.votedFor.Store(candidateHash)
	election.resetElectionTimer()

	return true
}

func (election *Election) handleHeartbeat(term uint64, leaderId string) bool {
	currentTerm := election.term.Load()

	if term > currentTerm {
		election.stepDownLocked(term)

		return true
	}

	if term < currentTerm {
		return false
	}

	if election.getState() != Leader && leaderId != "" {
		election.resetElectionTimer()
		election.node.metrics.termNumber.Store(term)
		election.node.metrics.SetNodeRole("follower", 0.0)

		return true
	}

	return false
}

func (election *Election) Close() {
	if !election.closed.CompareAndSwap(false, true) {
		return
	}

	if election.electionTimer != nil {
		if !election.electionTimer.Stop() {
			select {
			case <-election.electionTimer.C:
			default:
			}
		}
	}

	if election.heartbeatTimer != nil {
		if !election.heartbeatTimer.Stop() {
			select {
			case <-election.heartbeatTimer.C:
			default:
			}
		}
	}

	if election.voteRing != nil {
		election.voteRing.Close()
	}
}

func (election *Election) updateLogState(index uint64, term uint64) {
	for {
		currentIndex := election.lastLogIndex.Load()

		if index <= currentIndex {
			return
		}

		if election.lastLogIndex.CompareAndSwap(currentIndex, index) {
			election.lastLogTerm.Store(term)

			return
		}
	}
}

func (election *Election) getCurrentTerm() uint64 {
	return election.term.Load()
}

func (election *Election) getLastLogIndex() uint64 {
	return election.lastLogIndex.Load()
}

func (election *Election) getLastLogTerm() uint64 {
	return election.lastLogTerm.Load()
}

func (election *Election) schedule(
	id string,
	fn func(ctx context.Context) (any, error),
) {
	election.node.forest.pool.Schedule(
		"dmt/election/"+id,
		fn,
	)
}

func (election *Election) scheduleLoop(
	id string,
	fn func(ctx context.Context) (any, error),
) {
	election.node.forest.pool.Schedule(
		"dmt/election/"+id,
		fn,
		qpool.WithTTL(time.Second),
	)
}

// storeTermForTest sets the current term in tests.
func (election *Election) storeTermForTest(term uint64) {
	election.term.Store(term)
}

// storeVotedForForTest sets votedFor in tests.
func (election *Election) storeVotedForForTest(candidateId string) {
	if candidateId == "" {
		election.votedFor.Store(0)

		return
	}

	election.votedFor.Store(hashNodeID(candidateId))
}

// storeLogStateForTest sets log indices in tests.
func (election *Election) storeLogStateForTest(index uint64, term uint64) {
	election.lastLogIndex.Store(index)
	election.lastLogTerm.Store(term)
}

// votedForForTest returns votedFor in tests.
func (election *Election) votedForForTest() uint64 {
	return election.votedFor.Load()
}

// lastLogTermForTest returns lastLogTerm in tests.
func (election *Election) lastLogTermForTest() uint64 {
	return election.lastLogTerm.Load()
}
