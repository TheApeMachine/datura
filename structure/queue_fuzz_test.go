package structure

import (
	"context"
	"testing"
)

/*
FuzzSPSCRingFIFO verifies single-producer single-consumer FIFO order for arbitrary
push sequences when the ring never rejects overflow.
*/
func FuzzSPSCRingFIFO(f *testing.F) {
	f.Add([]byte("alpha"))
	f.Add([]byte("one-two-three"))

	f.Fuzz(func(t *testing.T, payload []byte) {
		if len(payload) == 0 {
			return
		}

		capacity := nextPowerOfTwo(len(payload))
		ring := NewSPSCRing[byte](capacity, false)

		if ring == nil {
			t.Fatalf("NewSPSCRing returned nil for capacity %d", capacity)
		}

		for _, value := range payload {
			if !ring.Push(value) {
				t.Fatalf("push rejected with capacity %d and len %d", capacity, len(payload))
			}
		}

		for index, expected := range payload {
			value, ok := ring.Pop()

			if !ok {
				t.Fatalf("pop failed at index %d", index)
			}

			if value != expected {
				t.Fatalf("fifo violation at %d: got %q want %q", index, value, expected)
			}
		}

		if _, ok := ring.Pop(); ok {
			t.Fatal("expected empty ring after draining fuzz input")
		}
	})
}

/*
FuzzMPMCRingFIFO verifies FIFO order for single-threaded push/pop sequences.
*/
func FuzzMPMCRingFIFO(f *testing.F) {
	f.Add([]byte("alpha"))
	f.Add([]byte("one-two-three"))

	f.Fuzz(func(t *testing.T, payload []byte) {
		if len(payload) == 0 {
			return
		}

		capacity := nextPowerOfTwo(max(len(payload), 2))
		ring, err := NewMPMCRing[byte](context.Background(), capacity)

		if err != nil {
			t.Fatalf("NewMPMCRing: %v", err)
		}

		for _, value := range payload {
			if !ring.Push(value) {
				t.Fatalf("push rejected with capacity %d and len %d", capacity, len(payload))
			}
		}

		for index, expected := range payload {
			value, ok := ring.Pop()

			if !ok {
				t.Fatalf("pop failed at index %d", index)
			}

			if value != expected {
				t.Fatalf("fifo violation at %d: got %q want %q", index, value, expected)
			}
		}

		if _, ok := ring.Pop(); ok {
			t.Fatal("expected empty ring after draining fuzz input")
		}
	})
}

func nextPowerOfTwo(value int) int {
	if value < 1 {
		return 1
	}

	capacity := 1

	for capacity < value {
		capacity <<= 1
	}

	return capacity
}

func max(left, right int) int {
	if left > right {
		return left
	}

	return right
}
