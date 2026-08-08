package dmt

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestEditDistance(t *testing.T) {
	Convey("Given token pairs", t, func() {
		Convey("Then identical tokens are zero apart", func() {
			So(EditDistance([]byte("frenzy"), []byte("frenzy")), ShouldEqual, 0)
		})

		Convey("Then a substitution costs one", func() {
			So(EditDistance([]byte("frenzy"), []byte("frenzz")), ShouldEqual, 1)
		})

		Convey("Then a deletion costs one", func() {
			So(EditDistance([]byte("frenzy"), []byte("frezy")), ShouldEqual, 1)
		})

		Convey("Then an insertion costs one", func() {
			So(EditDistance([]byte("frenzy"), []byte("frenzyy")), ShouldEqual, 1)
		})

		Convey("Then an empty side costs the other's length", func() {
			So(EditDistance(nil, []byte("abc")), ShouldEqual, 3)
			So(EditDistance([]byte("abc"), nil), ShouldEqual, 3)
		})

		Convey("Then it is symmetric", func() {
			So(
				EditDistance([]byte("laminar"), []byte("turbulent")),
				ShouldEqual,
				EditDistance([]byte("turbulent"), []byte("laminar")),
			)
		})
	})
}

func TestNgramSimilarity(t *testing.T) {
	Convey("Given token pairs", t, func() {
		Convey("Then a token is identical to itself", func() {
			So(
				NgramSimilarity([]byte("absorption"), []byte("absorption")),
				ShouldAlmostEqual,
				1.0,
				1e-9,
			)
		})

		Convey("Then a near neighbour scores above an unrelated token", func() {
			near := NgramSimilarity([]byte("absorption"), []byte("absorbtion"))
			far := NgramSimilarity([]byte("absorption"), []byte("xqzw"))
			So(near, ShouldBeGreaterThan, far)
		})

		Convey("Then an empty token has no similarity", func() {
			So(NgramSimilarity(nil, []byte("abc")), ShouldEqual, 0)
		})

		Convey("Then padding distinguishes position", func() {
			// Same characters, different arrangement: the sentinels stop these
			// reading as the same profile.
			So(
				NgramSimilarity([]byte("ab"), []byte("ba")),
				ShouldBeLessThan,
				1.0,
			)
		})
	})
}

func TestVocabularyAndCoOccurrence(t *testing.T) {
	Convey("Given a trained sequence", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)

		tree.TrainSensorySequence([]byte("coil_ignition_exhaustion"))

		Convey("Then its tokens enter the vocabulary", func() {
			So(tree.KnowsToken([]byte("coil")), ShouldBeTrue)
			So(tree.KnowsToken([]byte("ignition")), ShouldBeTrue)
			So(tree.KnowsToken([]byte("never_seen")), ShouldBeFalse)
		})

		Convey("Then neighbours within the window are tallied", func() {
			So(
				tree.CoOccurrenceCount([]byte("coil"), []byte("ignition")),
				ShouldBeGreaterThan,
				0,
			)
		})

		Convey("Then a token is not counted as its own neighbour", func() {
			So(
				tree.CoOccurrenceCount([]byte("coil"), []byte("coil")),
				ShouldEqual,
				0,
			)
		})
	})
}

func TestResolveToken(t *testing.T) {
	Convey("Given a model with a known vocabulary", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)

		tree.TrainSensorySequence([]byte("exhaustion_laminar"))

		Convey("When the token is known", func() {
			match := tree.ResolveToken([]byte("laminar"))

			Convey("Then it resolves to itself at certainty", func() {
				So(string(match.Mapped), ShouldEqual, "laminar")
				So(match.Similarity, ShouldEqual, 1)
				So(match.Known, ShouldBeTrue)
			})
		})

		Convey("When the token is one edit away", func() {
			match := tree.ResolveToken([]byte("laminaz"))

			Convey("Then it maps onto the known token with high confidence", func() {
				So(string(match.Mapped), ShouldEqual, "laminar")
				So(match.Similarity, ShouldBeGreaterThan, 0.9)
			})
		})

		Convey("When the token is unrelated", func() {
			match := tree.ResolveToken([]byte("zzzzzzzz"))

			Convey("Then whatever it maps to is reported at low similarity", func() {
				So(match.Similarity, ShouldBeLessThan, 0.5)
			})
		})

		Convey("When the token is empty", func() {
			match := tree.ResolveToken(nil)

			Convey("Then it claims nothing", func() {
				So(match.Known, ShouldBeFalse)
			})
		})
	})
}
