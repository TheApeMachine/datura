package dmt

import (
	"testing"
)

func TestHashNodeID(t *testing.T) {
	first := hashNodeID("node-a")
	second := hashNodeID("node-a")
	third := hashNodeID("node-b")

	if first == 0 {
		t.Fatal("expected non-zero node id hash")
	}

	if first != second {
		t.Fatal("expected stable node id hash")
	}

	if first == third {
		t.Fatal("expected distinct node id hashes")
	}
}
