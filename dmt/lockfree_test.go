package dmt

import (
	"context"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura/structure"
)

func TestPeerRegistryConcurrentUpsert(test *testing.T) {
	Convey("Given concurrent peer registration", test, func() {
		registry := newPeerRegistry()

		var waitGroup sync.WaitGroup
		errors := make(chan error, 16)

		for workerIndex := range 16 {
			waitGroup.Add(1)

			go func(index int) {
				defer waitGroup.Done()

				addr := "peer-" + string(rune('a'+index))
				registry.Upsert(addr, &peer{addr: addr})
			}(workerIndex)
		}

		waitGroup.Wait()
		close(errors)

		for registryErr := range errors {
			So(registryErr, ShouldBeNil)
		}

		Convey("It should retain every registered peer", func() {
			So(registry.Load().Len(), ShouldEqual, 16)
		})
	})
}

func TestElectionVoteRing(test *testing.T) {
	Convey("Given an election vote ring", test, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		voteRing, err := structure.NewMPMCRing[string](ctx, 128)
		So(err, ShouldBeNil)
		defer voteRing.Close()

		election := &Election{
			voteRing: voteRing,
			node:     &NetworkNode{metrics: NewMetrics()},
		}

		Convey("When publishing votes", func() {
			election.role.Store(uint32(Candidate))
			election.publishVote("peer-a")
			election.publishVote("peer-b")
			election.drainVotes()

			Convey("It should count granted votes", func() {
				So(election.votesReceived.Load(), ShouldEqual, 2)
			})
		})
	})
}
