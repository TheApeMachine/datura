package dmt

import (
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestInterpolatedSurprisalIsFiniteOnNovelty(t *testing.T) {
	Convey("Given a model with a learned transition", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)

		for range 10 {
			tree.TrainSensorySequence([]byte("coil_ignition"))
		}

		Convey("When scoring the familiar sequence", func() {
			items := tree.InterpolatedSurprisal([]byte("coil_ignition"))

			Convey("Then every token is finite", func() {
				So(len(items), ShouldEqual, 2)

				for _, item := range items {
					So(item.Surprisal, ShouldBeGreaterThanOrEqualTo, 0)
					So(item.Surprisal, ShouldBeLessThan, 20)
				}
			})
		})

		Convey("When scoring a sequence the model has never seen", func() {
			items := tree.InterpolatedSurprisal([]byte("coil_catastrophe"))

			Convey("Then it is surprising rather than infinite", func() {
				So(len(items), ShouldEqual, 2)

				for _, item := range items {
					So(item.Surprisal, ShouldBeLessThan, 20)
				}
			})

			Convey("Then the novel continuation is the more surprising one", func() {
				familiar := tree.InterpolatedSurprisal([]byte("coil_ignition"))
				So(items[1].Surprisal, ShouldBeGreaterThan, familiar[1].Surprisal)
			})
		})
	})
}

func TestExperienceSpawnsAConceptWhenNothingExplains(t *testing.T) {
	Convey("Given a model that knows nothing", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)

		var scratch ClassificationScratch

		Convey("When it experiences a sequence unsupervised", func() {
			outcome, experienceErr := tree.ExperienceSequence(
				[]byte("alpha_beta"), &scratch,
			)

			Convey("Then it names a concept for itself", func() {
				So(experienceErr, ShouldBeNil)
				So(outcome.NewConcept, ShouldBeTrue)
				So(strings.HasPrefix(string(outcome.Class), "concept_"), ShouldBeTrue)
			})

			Convey("Then the concept is registered as a known class", func() {
				classes := tree.KnownClasses()
				So(len(classes), ShouldBeGreaterThan, 0)
			})
		})
	})
}

func TestExperienceReusesAnExplainingClass(t *testing.T) {
	Convey("Given a model taught one class thoroughly", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)

		var scratch ClassificationScratch

		for range 12 {
			So(tree.TeachSequence([]byte("drive_ignition"), []byte("bullish")), ShouldBeNil)
		}

		before := len(tree.KnownClasses())

		Convey("When it re-experiences the same sequence", func() {
			outcome, experienceErr := tree.ExperienceSequence(
				[]byte("drive_ignition"), &scratch,
			)

			Convey("Then it claims the existing class rather than inventing one", func() {
				So(experienceErr, ShouldBeNil)
				So(outcome.NewConcept, ShouldBeFalse)
				So(string(outcome.Class), ShouldEqual, "bullish")
			})

			Convey("Then no new class appeared", func() {
				So(len(tree.KnownClasses()), ShouldEqual, before)
			})
		})
	})
}

func TestExperienceRejectsEmptySequences(t *testing.T) {
	Convey("Given a model", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)

		var scratch ClassificationScratch

		Convey("Then an empty sequence is an error, not a spawned concept", func() {
			_, experienceErr := tree.ExperienceSequence(nil, &scratch)
			So(experienceErr, ShouldEqual, ErrEmptySequence)
		})
	})
}

func TestContrastiveTokenContributions(t *testing.T) {
	Convey("Given two classes separated by one transition", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)

		for range 10 {
			_ = tree.TeachSequence([]byte("drive_ignition"), []byte("bullish"))
			_ = tree.TeachSequence([]byte("drive_absorption"), []byte("bearish"))
		}

		Convey("When explaining a sequence", func() {
			contributions := tree.ContrastiveTokenContributions(
				[]byte("drive_ignition"),
			)

			Convey("Then every token is accounted for", func() {
				So(len(contributions), ShouldEqual, 2)
			})

			Convey("Then the shared token is less decisive than the distinguishing one", func() {
				shared := contributions[0].Bits
				distinguishing := contributions[1].Bits
				So(distinguishing, ShouldNotAlmostEqual, shared, 1e-9)
			})
		})

		Convey("When only one class exists", func() {
			single, singleErr := NewTree("")
			So(singleErr, ShouldBeNil)

			_ = single.TeachSequence([]byte("a_b"), []byte("only"))

			Convey("Then there is nothing to contrast against", func() {
				So(single.ContrastiveTokenContributions([]byte("a_b")), ShouldBeNil)
			})
		})
	})
}

func TestKnownClassesDeduplicates(t *testing.T) {
	Convey("Given a class recorded in both the concept register and basins", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)

		_ = tree.TeachSequence([]byte("a_b"), []byte("shared"))
		tree.registerClass([]byte("shared"))

		Convey("Then it is listed once", func() {
			count := 0

			for _, class := range tree.KnownClasses() {
				if string(class) == "shared" {
					count++
				}
			}

			So(count, ShouldEqual, 1)
		})
	})
}
