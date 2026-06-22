package datura

import "testing"

func TestWithPayloadAfterWrite(t *testing.T) {
	inbound := Acquire("inbound", APPJSON)
	inbound.Merge("features", []float64{1.0})

	packed, err := inbound.MarshalPacked()

	if err != nil {
		t.Fatal(err)
	}

	state := Acquire("state", APPJSON)

	if _, err = state.Write(packed); err != nil {
		t.Fatal(err)
	}

	output := Acquire("output", APPJSON)
	result := output.WithPayload(state.DecryptPayload())

	if result == nil {
		t.Fatal("WithPayload returned nil on fresh output after state Write")
	}

	if Peek[[]float64](output, "features")[0] != 1.0 {
		t.Fatalf("features = %v", Peek[[]float64](output, "features"))
	}
}

func TestHistoryPeekRoundTrip(t *testing.T) {
	config := Acquire("history-roundtrip", APPJSON)
	config.Merge("history", []float64{1, 2, 3})

	history := Peek[[]float64](config, "history")

	if len(history) != 3 {
		t.Fatalf("history = %v", history)
	}
}

func TestMergeOnFreshArtifact(t *testing.T) {
	artifact := Acquire("merge-fresh", APPJSON)
	artifact.Merge("features", []float64{1, 2, 3})

	features := Peek[[]float64](artifact, "features")

	if len(features) != 3 {
		t.Fatalf("features = %v", features)
	}
}

func TestToWithoutPayload(t *testing.T) {
	artifact := Acquire("to-fresh", APPJSON)

	err := artifact.To(&Map[any]{})

	if err == nil {
		t.Fatal("expected error unmarshalling artifact without payload")
	}
}
