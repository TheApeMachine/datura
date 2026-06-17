package datura

import (
	"testing"
	"time"

	"github.com/bytedance/sonic/ast"
	. "github.com/smartystreets/goconvey/convey"
)

const tradePayloadFixture = `[{"symbol":"BTC/USD","side":"buy","price":50000,"qty":0.1,"ord_type":"market","trade_id":1,"timestamp":"2024-01-01T00:00:00Z"}]`

func tradeArtifact(testingTB testing.TB) *Artifact {
	testingTB.Helper()

	artifact := Acquire("payload-test", Artifact_Type_json).
		WithPayload([]byte(tradePayloadFixture))

	if artifact == nil {
		testingTB.Fatal("Acquire returned nil")
	}

	return artifact
}

func TestPeekPayloadOK(t *testing.T) {
	Convey("Given a trade array payload", t, func() {
		artifact := tradeArtifact(t)

		defer artifact.Release()

		Convey("It should read scalar fields by path", func() {
			symbol, symbolOK := PeekPayloadOK[string](artifact, "0.symbol")
			So(symbolOK, ShouldBeTrue)
			So(symbol, ShouldEqual, "BTC/USD")

			price, priceOK := PeekPayloadOK[float64](artifact, "0.price")
			So(priceOK, ShouldBeTrue)
			So(price, ShouldEqual, 50000)

			side, sideOK := PeekPayloadOK[string](artifact, "0.side")
			So(sideOK, ShouldBeTrue)
			So(side, ShouldEqual, "buy")
		})

		Convey("It should report false for missing paths", func() {
			_, absentOK := PeekPayloadOK[string](artifact, "0.missing")
			So(absentOK, ShouldBeFalse)
		})
	})
}

func TestPayloadLen(t *testing.T) {
	Convey("Given a trade array payload", t, func() {
		artifact := tradeArtifact(t)

		defer artifact.Release()

		Convey("It should return the root array length", func() {
			length, lengthOK := PayloadLen(artifact)
			So(lengthOK, ShouldBeTrue)
			So(length, ShouldEqual, 1)
		})
	})
}

func TestPayloadEach(t *testing.T) {
	Convey("Given a trade array payload", t, func() {
		artifact := tradeArtifact(t)

		defer artifact.Release()

		Convey("It should visit each element without unmarshaling structs", func() {
			var (
				visited int
				symbol  string
			)

			PayloadEach(artifact, func(index int, element ast.Node) bool {
				So(index, ShouldEqual, visited)

				value, err := element.Get("symbol").String()
				So(err, ShouldBeNil)
				symbol = value
				visited++

				return true
			})

			So(visited, ShouldEqual, 1)
			So(symbol, ShouldEqual, "BTC/USD")
		})
	})
}

func TestPeekPayloadCache(t *testing.T) {
	Convey("Given repeated payload peeks", t, func() {
		artifact := tradeArtifact(t)

		defer artifact.Release()

		Convey("It should reuse the parsed root across lookups", func() {
			first, firstOK := PeekPayloadOK[float64](artifact, "0.price")
			So(firstOK, ShouldBeTrue)

			state := artifactStreamStateFor(artifact)
			So(state.payloadParsed, ShouldBeTrue)

			second, secondOK := PeekPayloadOK[float64](artifact, "0.qty")
			So(secondOK, ShouldBeTrue)
			So(first, ShouldEqual, 50000)
			So(second, ShouldEqual, 0.1)
		})
	})
}

func TestPayloadQuiet(testingTB *testing.T) {
	Convey("Given an artifact without encrypted payload metadata", testingTB, func() {
		artifact := Acquire("payload-quiet", Artifact_Type_json)

		if artifact == nil {
			testingTB.Fatal("Acquire returned nil")
		}

		defer artifact.Release()

		Convey("PayloadQuiet should return false without error logging", func() {
			payload, payloadOK := artifact.PayloadQuiet()
			So(payloadOK, ShouldBeFalse)
			So(payload, ShouldBeNil)
		})
	})

	Convey("Given an artifact with encrypted payload metadata", testingTB, func() {
		artifact := tradeArtifact(testingTB)

		defer artifact.Release()

		Convey("PayloadQuiet should decrypt the payload", func() {
			payload, payloadOK := artifact.PayloadQuiet()
			So(payloadOK, ShouldBeTrue)
			So(len(payload), ShouldBeGreaterThan, 0)
		})
	})
}

func BenchmarkPeekPayloadOK(b *testing.B) {
	b.ResetTimer()

	for b.Loop() {
		artifact := tradeArtifact(b)
		price, priceOK := PeekPayloadOK[float64](artifact, "0.price")
		artifact.Release()

		if !priceOK || price == 0 {
			b.Fatal("peek payload missed price")
		}
	}
}

func BenchmarkAsTradeUpdates(b *testing.B) {
	type tradeUpdate struct {
		Symbol    string    `json:"symbol"`
		Side      string    `json:"side"`
		Price     float64   `json:"price"`
		Qty       float64   `json:"qty"`
		Timestamp time.Time `json:"timestamp"`
	}

	b.ResetTimer()

	for b.Loop() {
		artifact := tradeArtifact(b)
		updates := As[[]*tradeUpdate](artifact)
		artifact.Release()

		if len(updates) == 0 || updates[0].Price == 0 {
			b.Fatal("as trade updates missed price")
		}
	}
}
