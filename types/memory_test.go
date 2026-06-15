package types

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewMemory(t *testing.T) {
	Convey("NewMemory returns empty slices", t, func() {
		memory := NewMemory()
		So(memory.Documents, ShouldBeEmpty)
		So(memory.Embeddings, ShouldBeEmpty)
		So(memory.Relationships, ShouldBeEmpty)
	})
}

func TestMemoryAddDocument(t *testing.T) {
	Convey("AddDocument appends to Documents", t, func() {
		memory := NewMemory()
		memory.AddDocument(Document{ID: "a", Text: "alpha"})
		memory.AddDocument(Document{ID: "b", Text: "beta"})

		So(len(memory.Documents), ShouldEqual, 2)
		So(memory.Documents[0].ID, ShouldEqual, "a")
		So(memory.Documents[1].Text, ShouldEqual, "beta")
	})
}

func TestMemoryAddEmbedding(t *testing.T) {
	Convey("AddEmbedding appends to Embeddings", t, func() {
		memory := NewMemory()
		memory.AddEmbedding(Embedding{ID: "e1", Embedding: []float32{1, 2}})

		So(len(memory.Embeddings), ShouldEqual, 1)
		So(memory.Embeddings[0].ID, ShouldEqual, "e1")
	})
}

func TestMemoryAddRelationship(t *testing.T) {
	Convey("AddRelationship appends to Relationships", t, func() {
		memory := NewMemory()
		memory.AddRelationship(Relationship{ID: "r1", Relationship: "LINKS", ToID: "b"})

		So(len(memory.Relationships), ShouldEqual, 1)
		So(memory.Relationships[0].ToID, ShouldEqual, "b")
	})
}

func TestMemoryMerge(t *testing.T) {
	Convey("Merge combines documents, embeddings, and relationships", t, func() {
		left := NewMemory()
		left.AddDocument(Document{ID: "a"})
		left.AddEmbedding(Embedding{ID: "e1"})

		right := NewMemory()
		right.AddDocument(Document{ID: "b"})
		right.AddRelationship(Relationship{ID: "r1"})
		right.Metadata = Metadata{ID: "meta", Source: "src", Timestamp: time.Unix(1, 0)}

		left.Merge(right)

		So(len(left.Documents), ShouldEqual, 2)
		So(len(left.Embeddings), ShouldEqual, 1)
		So(len(left.Relationships), ShouldEqual, 1)
		So(left.Metadata.ID, ShouldEqual, "meta")
		So(left.Metadata.Source, ShouldEqual, "src")
	})

	Convey("Merge skips empty metadata on other", t, func() {
		left := NewMemory()
		left.Metadata = Metadata{ID: "keep"}

		right := NewMemory()
		left.Merge(right)

		So(left.Metadata.ID, ShouldEqual, "keep")
	})
}

func BenchmarkMemoryMerge(b *testing.B) {
	leftDocs := make([]Document, 128)
	rightDocs := make([]Document, 128)

	for index := range leftDocs {
		leftDocs[index] = Document{ID: "l", Text: "left"}
		rightDocs[index] = Document{ID: "r", Text: "right"}
	}

	b.ResetTimer()

	for b.Loop() {
		left := NewMemory()
		left.Documents = append(left.Documents[:0:0], leftDocs...)

		right := NewMemory()
		right.Documents = append(right.Documents[:0:0], rightDocs...)
		right.Metadata = Metadata{ID: "meta"}

		left.Merge(right)

		if len(left.Documents) != 256 {
			b.Fatalf("expected 256 documents, got %d", len(left.Documents))
		}
	}
}
