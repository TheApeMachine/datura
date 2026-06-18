package datura

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

const tradePayloadFixture = `[{"symbol":"BTC/USD","side":"buy","price":50000,"qty":0.1,"ord_type":"market","trade_id":1,"timestamp":"2024-01-01T00:00:00Z"}]`

func tradeArtifact(t testing.TB) *Artifact {
	t.Helper()

	artifact := Acquire("payload-test", Artifact_Type_json).
		WithPayload([]byte(tradePayloadFixture))

	if artifact == nil {
		t.Fatal("Acquire returned nil")
	}

	return artifact
}

func TestPeekPayload(t *testing.T) {
	Convey("Given a trade array payload", t, func() {
		artifact := tradeArtifact(t)

		defer artifact.Release()

		Convey("It should read scalar fields by path", func() {
			So(PeekPayload[string](artifact, "0.symbol"), ShouldEqual, "BTC/USD")
			So(PeekPayload[float64](artifact, "0.price"), ShouldEqual, 50000)
			So(PeekPayload[string](artifact, "0.side"), ShouldEqual, "buy")
		})

	})

	Convey("Given a nested object payload", t, func() {
		artifact := Acquire("payload-object-test", Artifact_Type_json).
			WithPayload([]byte(`{"data":{"price":[{"label":"price","value":10.5,"transform":"ema"}]}}`))

		defer artifact.Release()

		Convey("It should read object nodes as map[string]any", func() {
			field := PeekPayload[map[string]any](artifact, "data.price.0")

			So(field["label"], ShouldEqual, "price")
			So(field["value"], ShouldEqual, 10.5)
			So(field["transform"], ShouldEqual, "ema")
		})

		Convey("It should read object nodes as map[string]any", func() {
			field := PeekPayload[map[string]any](artifact, "data.price.0")

			So(field["label"], ShouldEqual, "price")
			So(field["value"], ShouldEqual, 10.5)
			So(field["transform"], ShouldEqual, "ema")
		})

		Convey("It should read scalar fields under nested paths", func() {
			So(PeekPayload[string](artifact, "data.price.0.label"), ShouldEqual, "price")
			So(PeekPayload[float64](artifact, "data.price.0.value"), ShouldEqual, 10.5)
		})
	})
}

func TestPayloadQuiet(t *testing.T) {
	Convey("Given an artifact without encrypted payload metadata", t, func() {
		artifact := Acquire("payload-quiet", Artifact_Type_json)

		defer artifact.Release()

		Convey("PayloadQuiet should return false without error logging", func() {
			payload, payloadOK := artifact.PayloadQuiet()
			So(payloadOK, ShouldBeFalse)
			So(payload, ShouldBeNil)
		})
	})

	Convey("Given an artifact with encrypted payload metadata", t, func() {
		artifact := tradeArtifact(t)

		defer artifact.Release()

		Convey("PayloadQuiet should decrypt the payload", func() {
			payload, payloadOK := artifact.PayloadQuiet()
			So(payloadOK, ShouldBeTrue)
			So(len(payload), ShouldBeGreaterThan, 0)
		})
	})
}

func BenchmarkPeekPayload(b *testing.B) {
	b.ResetTimer()

	for b.Loop() {
		artifact := tradeArtifact(b)
		price := PeekPayload[float64](artifact, "0.price")
		artifact.Release()

		if price == 0 {
			b.Fatal("peek payload missed price")
		}
	}
}

func BenchmarkPeekPayload_Object(b *testing.B) {
	payload := []byte(`{"data":{"price":[{"label":"price","value":10.5,"transform":"ema"}]}}`)

	b.ResetTimer()

	for b.Loop() {
		artifact := Acquire("payload-object-bench", Artifact_Type_json).WithPayload(payload)
		field := PeekPayload[map[string]any](artifact, "data.price.0")
		artifact.Release()

		if field["value"] == nil {
			b.Fatal("peek payload missed object field")
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
