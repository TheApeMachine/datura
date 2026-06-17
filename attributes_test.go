package datura

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPeekArtifactFields(t *testing.T) {
	Convey("Given artifact struct fields set with WithRole and WithScope", t, func() {
		artifact := Acquire("trader", Artifact_Type_json).
			WithRole("measurement").
			WithScope("BTC/USD").
			WithDestination("ui")

		Convey("It should resolve role and scope through Peek", func() {
			So(Peek[string](artifact, "role"), ShouldEqual, "measurement")
			So(Peek[string](artifact, "scope"), ShouldEqual, "BTC/USD")
			So(Peek[string](artifact, "destination"), ShouldEqual, "ui")
		})

		Convey("It should prefer metadata attributes over struct fields", func() {
			artifact.Poke("role", "gauge")

			So(Peek[string](artifact, "role"), ShouldEqual, "gauge")
		})
	})
}

func TestPeekOK(t *testing.T) {
	Convey("Given an artifact with typed metadata attributes", t, func() {
		artifact := Acquire("peek-ok", Artifact_Type_json)
		err := artifact.SetMetaValues(map[string]any{
			"count":   42,
			"ratio":   0.75,
			"enabled": true,
			"label":   "active",
			"missing": "not-a-number",
		})
		So(err, ShouldBeNil)

		Convey("It should return present keys with correct types in one pass", func() {
			count, countOK := PeekOK[int](artifact, "count")
			So(countOK, ShouldBeTrue)
			So(count, ShouldEqual, 42)

			ratio, ratioOK := PeekOK[float64](artifact, "ratio")
			So(ratioOK, ShouldBeTrue)
			So(ratio, ShouldEqual, 0.75)

			enabled, enabledOK := PeekOK[bool](artifact, "enabled")
			So(enabledOK, ShouldBeTrue)
			So(enabled, ShouldBeTrue)

			label, labelOK := PeekOK[string](artifact, "label")
			So(labelOK, ShouldBeTrue)
			So(label, ShouldEqual, "active")
		})

		Convey("It should report false when key is absent", func() {
			_, absentOK := PeekOK[string](artifact, "absent")
			So(absentOK, ShouldBeFalse)
		})

		Convey("It should report false when type does not match stored value", func() {
			_, mismatchOK := PeekOK[int](artifact, "label")
			So(mismatchOK, ShouldBeFalse)
		})
	})
}

func TestSetMetaValues(t *testing.T) {
	Convey("Given an artifact receiving multiple metadata entries at once", t, func() {
		artifact := Acquire("batch-meta", Artifact_Type_json)
		err := artifact.SetMetaValues(map[string]any{
			"alpha": "first",
			"beta":  2,
			"gamma": 3.5,
		})
		So(err, ShouldBeNil)

		Convey("It should expose all values through Peek", func() {
			So(Peek[string](artifact, "alpha"), ShouldEqual, "first")
			So(Peek[int](artifact, "beta"), ShouldEqual, 2)
			So(Peek[float64](artifact, "gamma"), ShouldEqual, 3.5)
		})

		Convey("It should append onto existing metadata with SetMetaValue", func() {
			err = artifact.SetMetaValue("delta", "fourth")
			So(err, ShouldBeNil)
			So(Peek[string](artifact, "delta"), ShouldEqual, "fourth")
			So(Peek[string](artifact, "alpha"), ShouldEqual, "first")
		})
	})
}

func TestPeekEach(t *testing.T) {
	Convey("Given an artifact with several metadata attributes", t, func() {
		artifact := Acquire("peek-each", Artifact_Type_json)
		err := artifact.SetMetaValues(map[string]any{
			"role":      "measurement",
			"scope":     "BTC/USD",
			"threshold": 7,
		})
		So(err, ShouldBeNil)

		Convey("It should collect multiple fields in one pass", func() {
			var (
				role      string
				scope     string
				threshold int
			)

			artifact.PeekEach(func(key string, value Artifact_Attribute_value) bool {
				switch key {
				case "role":
					role, _ = value.TextValue()
				case "scope":
					scope, _ = value.TextValue()
				case "threshold":
					threshold = int(value.IntValue())
				}

				return true
			})

			So(role, ShouldEqual, "measurement")
			So(scope, ShouldEqual, "BTC/USD")
			So(threshold, ShouldEqual, 7)
		})

		Convey("It should stop when the callback returns false", func() {
			seen := 0

			artifact.PeekEach(func(key string, value Artifact_Attribute_value) bool {
				seen++
				return false
			})

			So(seen, ShouldEqual, 1)
		})
	})
}

func TestSetMetaValuesEmpty(t *testing.T) {
	Convey("Given an empty metadata map", t, func() {
		artifact := Acquire("empty-meta", Artifact_Type_json)

		Convey("It should no-op without error", func() {
			So(artifact.SetMetaValues(map[string]any{}), ShouldBeNil)
		})
	})
}

func BenchmarkPeekEach(b *testing.B) {
	b.ResetTimer()

	for b.Loop() {
		artifact := Acquire("bench-peek-each", Artifact_Type_json)
		_ = artifact.SetMetaValues(map[string]any{
			"role":       "measurement",
			"scope":      "BTC/USD",
			"confidence": 0.91,
			"category":   2,
			"surprise":   0.12,
		})

		var (
			role       string
			scope      string
			confidence float64
			category   int
			surprise   float64
		)

		artifact.PeekEach(func(key string, value Artifact_Attribute_value) bool {
			switch key {
			case "role":
				role, _ = value.TextValue()
			case "scope":
				scope, _ = value.TextValue()
			case "confidence":
				confidence = value.FloatValue()
			case "category":
				category = int(value.IntValue())
			case "surprise":
				surprise = value.FloatValue()
			}

			return true
		})

		if role+scope == "" {
			b.Fatal("peek each missed routing fields")
		}

		_ = confidence
		_ = category
		_ = surprise
	}
}

func TestArtifactIndexCache(t *testing.T) {
	Convey("Given a pooled artifact with metadata", t, func() {
		artifact := Acquire("index-cache", Artifact_Type_json)
		err := artifact.SetMetaValues(map[string]any{
			"alpha": "first",
			"beta":  2,
		})
		So(err, ShouldBeNil)

		Convey("It should lazily index on first PeekOK and serve subsequent lookups from cache", func() {
			first, firstOK := PeekOK[string](artifact, "alpha")
			So(firstOK, ShouldBeTrue)
			So(first, ShouldEqual, "first")

			state := artifactStreamStateFor(artifact)
			So(state.indexed, ShouldBeTrue)
			So(state.cache["alpha"], ShouldEqual, "first")
			So(state.cache["beta"], ShouldEqual, 2)

			second, secondOK := PeekOK[int](artifact, "beta")
			So(secondOK, ShouldBeTrue)
			So(second, ShouldEqual, 2)
		})

		Convey("It should keep cache hot when SetMetaValues runs after indexing", func() {
			_, _ = PeekOK[string](artifact, "alpha")
			err = artifact.SetMetaValues(map[string]any{"gamma": 3.5})
			So(err, ShouldBeNil)

			gamma, gammaOK := PeekOK[float64](artifact, "gamma")
			So(gammaOK, ShouldBeTrue)
			So(gamma, ShouldEqual, 3.5)
		})

		Reset(func() {
			artifact.Release()
		})
	})
}

func TestArtifactReleaseClearsCache(t *testing.T) {
	Convey("Given a released pooled artifact", t, func() {
		artifact := Acquire("release-cache", Artifact_Type_json)
		_ = artifact.SetMetaValue("stale", "value")
		_, _ = PeekOK[string](artifact, "stale")
		artifact.Release()

		Convey("It should not leak stale cache entries on the next acquire cycle", func() {
			fresh := Acquire("release-cache", Artifact_Type_json)
			So(fresh, ShouldNotBeNil)

			state := artifactStreamStateFor(fresh)
			So(state.indexed, ShouldBeFalse)
			So(len(state.cache), ShouldEqual, 0)

			_, staleOK := PeekOK[string](fresh, "stale")
			So(staleOK, ShouldBeFalse)

			fresh.Release()
		})
	})
}

func BenchmarkPeekOKMultiField(b *testing.B) {
	artifact := Acquire("bench-peek-each", Artifact_Type_json)
	_ = artifact.SetMetaValues(map[string]any{
		"role":       "measurement",
		"scope":      "BTC/USD",
		"confidence": 0.91,
		"category":   2,
		"surprise":   0.12,
	})
	defer artifact.Release()

	b.ResetTimer()

	for b.Loop() {
		role, _ := PeekOK[string](artifact, "role")
		scope, _ := PeekOK[string](artifact, "scope")
		confidence, _ := PeekOK[float64](artifact, "confidence")
		category, _ := PeekOK[int](artifact, "category")
		surprise, _ := PeekOK[float64](artifact, "surprise")

		if role+scope == "" {
			b.Fatal("peek ok missed routing fields")
		}

		_ = confidence
		_ = category
		_ = surprise
	}
}

func BenchmarkPeekOK(b *testing.B) {
	artifact := Acquire("bench-peek", Artifact_Type_json)
	_ = artifact.SetMetaValues(map[string]any{
		"k0": "v0",
		"k1": "v1",
		"k2": "v2",
		"k3": "v3",
		"k4": "target",
	})

	b.ResetTimer()

	for b.Loop() {
		_, _ = PeekOK[string](artifact, "target")
	}
}

func BenchmarkPeek(b *testing.B) {
	artifact := Acquire("bench-peek", Artifact_Type_json)
	_ = artifact.SetMetaValues(map[string]any{
		"k0": "v0",
		"k1": "v1",
		"k2": "v2",
		"k3": "v3",
		"k4": "target",
	})

	b.ResetTimer()

	for b.Loop() {
		_ = Peek[string](artifact, "target")
	}
}

func BenchmarkSetMetaValues(b *testing.B) {
	for b.Loop() {
		artifact := Acquire("bench-set", Artifact_Type_json)
		_ = artifact.SetMetaValues(map[string]any{
			"alpha":   "first",
			"beta":    2,
			"gamma":   3.5,
			"delta":   true,
			"epsilon": int64(99),
		})
	}
}

func BenchmarkSetMetaValueSequential(b *testing.B) {
	for b.Loop() {
		artifact := Acquire("bench-set", Artifact_Type_json)
		_ = artifact.SetMetaValue("alpha", "first")
		_ = artifact.SetMetaValue("beta", 2)
		_ = artifact.SetMetaValue("gamma", 3.5)
		_ = artifact.SetMetaValue("delta", true)
		_ = artifact.SetMetaValue("epsilon", int64(99))
	}
}
