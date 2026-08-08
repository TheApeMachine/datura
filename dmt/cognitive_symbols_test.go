package dmt

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func symbolFor(symbols []DiscriminativeSymbol, path string) (DiscriminativeSymbol, bool) {
	for _, symbol := range symbols {
		if string(symbol.Symbol) == path {
			return symbol, true
		}
	}

	return DiscriminativeSymbol{}, false
}

func TestExtractDiscriminativeSymbols(t *testing.T) {
	Convey("Given one path unique to a class and one shared by both", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)

		for range 10 {
			// "shared" occurs under both classes; "unique" only under bullish.
			_ = tree.TeachSequence([]byte("shared"), []byte("bullish"))
			_ = tree.TeachSequence([]byte("shared"), []byte("bearish"))
			_ = tree.TeachSequence([]byte("unique"), []byte("bullish"))
		}

		symbols := tree.ExtractDiscriminativeSymbols(32)

		Convey("Then the class-unique path is extracted", func() {
			unique, found := symbolFor(symbols, "unique")
			So(found, ShouldBeTrue)
			So(string(unique.Class), ShouldEqual, "bullish")
			So(unique.Purity, ShouldAlmostEqual, 1.0, 1e-9)
		})

		Convey("Then the evenly shared path is not called discriminative", func() {
			_, found := symbolFor(symbols, "shared")
			So(found, ShouldBeFalse)
		})

		Convey("Then results are ordered by descending score", func() {
			for index := 1; index < len(symbols); index++ {
				So(
					symbols[index-1].Score,
					ShouldBeGreaterThanOrEqualTo,
					symbols[index].Score,
				)
			}
		})
	})
}

func TestSymbolExtractionRespectsEvidenceFloor(t *testing.T) {
	Convey("Given a path seen exactly once", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)

		_ = tree.TeachSequence([]byte("coincidence"), []byte("bullish"))

		Convey("Then a single association is not yet a symbol", func() {
			_, found := symbolFor(
				tree.ExtractDiscriminativeSymbols(32), "coincidence",
			)
			So(found, ShouldBeFalse)
		})
	})
}

func TestSymbolExtractionRewardsSpecificity(t *testing.T) {
	Convey("Given a short and a long path of equal purity and weight", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)

		// A second class is required for anything to be discriminative at all:
		// with one class every path is trivially pure and nothing distinguishes.
		for range 8 {
			_ = tree.TeachSequence([]byte("short"), []byte("bullish"))
			_ = tree.TeachSequence([]byte("a_b_c_long"), []byte("bullish"))
			_ = tree.TeachSequence([]byte("other"), []byte("bearish"))
		}

		symbols := tree.ExtractDiscriminativeSymbols(64)
		short, shortFound := symbolFor(symbols, "short")
		long, longFound := symbolFor(symbols, "a_b_c_long")

		Convey("Then both are extracted", func() {
			So(shortFound, ShouldBeTrue)
			So(longFound, ShouldBeTrue)
		})

		Convey("Then the longer commitment about context scores higher", func() {
			So(long.Score, ShouldBeGreaterThan, short.Score)
		})
	})
}

func TestSymbolExtractionBounds(t *testing.T) {
	Convey("Given a model with several symbols", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)

		for range 6 {
			_ = tree.TeachSequence([]byte("one"), []byte("bullish"))
			_ = tree.TeachSequence([]byte("two"), []byte("bullish"))
			_ = tree.TeachSequence([]byte("three"), []byte("bearish"))
		}

		Convey("Then the limit is respected", func() {
			So(len(tree.ExtractDiscriminativeSymbols(1)), ShouldEqual, 1)
		})

		Convey("Then a non-positive limit yields nothing", func() {
			So(tree.ExtractDiscriminativeSymbols(0), ShouldBeNil)
		})
	})

	Convey("Given a model that knows no classes", t, func() {
		tree, err := NewTree("")
		So(err, ShouldBeNil)

		Convey("Then there is nothing to discriminate", func() {
			So(tree.ExtractDiscriminativeSymbols(8), ShouldBeNil)
		})
	})
}
