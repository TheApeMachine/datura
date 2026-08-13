package dmt

import (
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

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
