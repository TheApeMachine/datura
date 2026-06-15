package qdrant

import (
	"context"
	"testing"

	qc "github.com/qdrant/go-client/qdrant"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura/types"
)

func TestStoreAdapterValidation(t *testing.T) {
	Convey("Store adapter validation", t, func() {
		client, err := NewClient(Config{PoolSize: 1})
		So(err, ShouldBeNil)
		defer client.Close()

		store := NewStore(client)
		ctx := context.Background()

		Convey("Get requires query id", func() {
			_, err := store.Get(ctx, types.Query{})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "query id is required")
		})

		Convey("Put requires mutation id and embedding", func() {
			err := store.Put(ctx, types.Mutation{})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "mutation id is required")

			err = store.Put(ctx, types.Mutation{ID: "doc-1"})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "embedding is required")
		})

		Convey("Delete requires mutation id", func() {
			err := store.Delete(ctx, types.Mutation{})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "mutation id is required")
		})

		Convey("Search requires embedding", func() {
			_, err := store.Search(ctx, types.Query{})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "embedding is required")
		})
	})
}

func TestPointIDString(t *testing.T) {
	Convey("pointIDString", t, func() {
		So(pointIDString(nil), ShouldEqual, "")

		So(pointIDString(qc.NewID("uuid-1")), ShouldEqual, "uuid-1")
		So(pointIDString(qc.NewIDNum(42)), ShouldEqual, "42")
	})
}

func TestPayloadString(t *testing.T) {
	Convey("payloadString", t, func() {
		So(payloadString(nil, "text"), ShouldEqual, "")

		payload := map[string]*qc.Value{
			"text": qc.NewValueString("hello"),
		}

		So(payloadString(payload, "text"), ShouldEqual, "hello")
		So(payloadString(payload, "missing"), ShouldEqual, "")
	})
}

func TestMemoryFromRetrieved(t *testing.T) {
	Convey("memoryFromRetrieved maps points to documents", t, func() {
		points := []*qc.RetrievedPoint{
			{
				Id: qc.NewID("doc-1"),
				Payload: map[string]*qc.Value{
					"text": qc.NewValueString("alpha"),
				},
				Vectors: &qc.VectorsOutput{
					VectorsOptions: &qc.VectorsOutput_Vector{
						Vector: &qc.VectorOutput{
							Vector: &qc.VectorOutput_Dense{
								Dense: &qc.DenseVector{Data: []float32{0.1, 0.2}},
							},
						},
					},
				},
			},
		}

		memory := memoryFromRetrieved(points)
		So(len(memory.Documents), ShouldEqual, 1)
		So(memory.Documents[0].ID, ShouldEqual, "doc-1")
		So(memory.Documents[0].Text, ShouldEqual, "alpha")
		So(memory.Documents[0].Embedding, ShouldResemble, []float32{0.1, 0.2})
	})
}

func TestMemoryFromScored(t *testing.T) {
	Convey("memoryFromScored maps hits to documents", t, func() {
		hits := []*qc.ScoredPoint{
			{
				Id: qc.NewIDNum(7),
				Payload: map[string]*qc.Value{
					"text": qc.NewValueString("beta"),
				},
			},
		}

		memory := memoryFromScored(hits)
		So(len(memory.Documents), ShouldEqual, 1)
		So(memory.Documents[0].ID, ShouldEqual, "7")
		So(memory.Documents[0].Text, ShouldEqual, "beta")
	})
}

func BenchmarkMemoryFromRetrieved(b *testing.B) {
	points := make([]*qc.RetrievedPoint, 128)

	for index := range points {
		points[index] = &qc.RetrievedPoint{
			Id: qc.NewID("doc"),
			Payload: map[string]*qc.Value{
				"text": qc.NewValueString("sample text"),
			},
			Vectors: &qc.VectorsOutput{
				VectorsOptions: &qc.VectorsOutput_Vector{
					Vector: &qc.VectorOutput{
						Vector: &qc.VectorOutput_Dense{
							Dense: &qc.DenseVector{Data: []float32{0.1, 0.2, 0.3}},
						},
					},
				},
			},
		}
	}

	b.ResetTimer()

	for b.Loop() {
		memory := memoryFromRetrieved(points)
		if len(memory.Documents) != 128 {
			b.Fatalf("expected 128 documents, got %d", len(memory.Documents))
		}
	}
}
