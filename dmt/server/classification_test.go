package server

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura/dmt"
)

func TestEvaluateClassification(t *testing.T) {
	Convey("Given a forest server with attractor basins", t, func() {
		ctx := context.Background()
		forest, err := dmt.NewForest(dmt.ForestConfig{})
		So(err, ShouldBeNil)

		tree := forest.GetFastestTree()
		_, _, _ = tree.InsertAttractorBasin(
			[]byte("Concept_3"),
			[]byte("the_blue"),
			dmt.CognitiveState{Count: 12, Probability: 0.737},
		)

		server := NewForestServer(WithContext(ctx), WithForest(forest))
		defer server.Close()

		Convey("When evaluating classification for a probe sequence", func() {
			result, evalErr := server.EvaluateClassification([]byte("the_blue"))

			Convey("Then it should return the dominant concept", func() {
				So(evalErr, ShouldBeNil)
				So(string(result.Winner), ShouldEqual, "Concept_3")
				So(result.Highest, ShouldBeGreaterThan, 0)
			})
		})
	})
}
