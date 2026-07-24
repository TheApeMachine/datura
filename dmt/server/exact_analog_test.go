package server

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura/dmt"
)

func TestExactLookupDoesNotFallback(t *testing.T) {
	Convey("Given a forest server with a structural sibling only", t, func() {
		ctx := context.Background()
		forest, err := dmt.NewForest(dmt.ForestConfig{})
		So(err, ShouldBeNil)

		knownKey := []byte("blue_cab_big")
		unknownKey := []byte("blue_drone_rotor")

		So(forest.Insert(knownKey, []byte("exact-payload")), ShouldBeNil)

		server, err := NewForestServer(WithContext(ctx), WithForest(forest))
		So(err, ShouldBeNil)
		defer server.Close()

		Convey("When exact lookup requests the unknown key", func() {
			value, found := server.ExactLookup(unknownKey)

			Convey("Then it should miss without analog fallback", func() {
				So(found, ShouldBeFalse)
				So(value, ShouldBeNil)
			})
		})

		Convey("When exact lookup requests the known key", func() {
			value, found := server.ExactLookup(knownKey)

			Convey("Then it should return the stored payload", func() {
				So(found, ShouldBeTrue)
				So(string(value), ShouldEqual, "exact-payload")
			})
		})
	})
}

func TestAnalogLookupReturnsMatchedKey(t *testing.T) {
	Convey("Given a forest server with a structural sibling", t, func() {
		ctx := context.Background()
		forest, err := dmt.NewForest(dmt.ForestConfig{})
		So(err, ShouldBeNil)

		knownKey := []byte("blue_cab_big")
		unknownKey := []byte("blue_drone_rotor")

		So(forest.Insert(knownKey, []byte("analog-payload")), ShouldBeNil)

		server, err := NewForestServer(WithContext(ctx), WithForest(forest))
		So(err, ShouldBeNil)
		defer server.Close()

		Convey("When analog lookup resolves the unknown key", func() {
			matchedKey, value, found := server.AnalogLookup(unknownKey)

			Convey("Then it should return the closest key and payload", func() {
				So(found, ShouldBeTrue)
				So(string(matchedKey), ShouldEqual, string(knownKey))
				So(string(value), ShouldEqual, "analog-payload")
			})
		})
	})
}

func TestForestServerClientReturnsLocalCapability(t *testing.T) {
	Convey("Given a forest server", t, func() {
		ctx := context.Background()
		server, err := NewForestServer(WithContext(ctx))
		So(err, ShouldBeNil)
		defer server.Close()

		Convey("Client should return the same local capability for in-process use", func() {
			first := server.Client("alpha")
			second := server.Client("beta")

			So(first.IsValid(), ShouldBeTrue)
			So(second.IsValid(), ShouldBeTrue)
		})
	})
}
