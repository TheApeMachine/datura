package deeplake

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFormatFloat4ArrayLiteral(t *testing.T) {
	Convey("FormatFloat4ArrayLiteral", t, func() {
		Convey("empty slice is {}", func() {
			So(FormatFloat4ArrayLiteral(nil), ShouldEqual, "{}")
			So(FormatFloat4ArrayLiteral([]float32{}), ShouldEqual, "{}")
		})

		Convey("single element", func() {
			So(FormatFloat4ArrayLiteral([]float32{0.5}), ShouldEqual, "{0.5}")
		})

		Convey("multiple elements are comma-separated", func() {
			So(FormatFloat4ArrayLiteral([]float32{1, 2, 3}), ShouldEqual, "{1,2,3}")
		})
	})
}

func TestStore_constructors(t *testing.T) {
	Convey("Store constructors", t, func() {
		client, err := NewClient(Config{APIKey: "k", OrgID: "o", Workspace: "ws"})
		So(err, ShouldBeNil)

		Convey("NewStore uses knowledge_base", func() {
			store := NewStore(client)
			So(store, ShouldNotBeNil)
			// exercise through CreateTable SQL shape below
		})

		Convey("NewStoreWithTable overrides table name", func() {
			store := NewStoreWithTable(client, "other")
			So(store, ShouldNotBeNil)
		})
	})
}

func TestStore_CreateTable(t *testing.T) {
	Convey("Store.CreateTable", t, func() {
		var received TableQueryBody

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			bodyBytes, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(bodyBytes, &received)
			testRespondJSON(w, http.StatusOK, []byte(`{}`))
		}))
		defer srv.Close()

		client, err := NewClient(Config{
			BaseURL:   srv.URL,
			APIKey:    "k",
			OrgID:     "o",
			Workspace: "myschema",
		})
		So(err, ShouldBeNil)

		store := NewStore(client)
		err = store.CreateTable(context.Background())
		So(err, ShouldBeNil)

		So(received.Query, ShouldContainSubstring, `CREATE TABLE IF NOT EXISTS`)
		So(received.Query, ShouldContainSubstring, `"myschema"`)
		So(received.Query, ShouldContainSubstring, `"knowledge_base"`)
		So(received.Query, ShouldContainSubstring, `USING deeplake`)
		So(received.Query, ShouldContainSubstring, `embedding FLOAT4[]`)
	})
}

func TestStore_InsertDocument(t *testing.T) {
	Convey("Store.InsertDocument", t, func() {
		var received TableQueryBody

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			bodyBytes, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(bodyBytes, &received)
			testRespondJSON(w, http.StatusOK, []byte(`{}`))
		}))
		defer srv.Close()

		client, err := NewClient(Config{
			BaseURL:   srv.URL,
			APIKey:    "k",
			OrgID:     "o",
			Workspace: "w",
		})
		So(err, ShouldBeNil)

		store := NewStoreWithTable(client, "docs")

		Convey("sends text, float4 literal, and jsonb metadata", func() {
			err := store.InsertDocument(context.Background(), "hello", []float32{0.25, 0.75}, map[string]any{"a": 1})
			So(err, ShouldBeNil)
			So(received.Query, ShouldContainSubstring, `INSERT INTO "w"."docs"`)
			So(len(received.Params), ShouldEqual, 3)
			So(received.Params[0], ShouldEqual, "hello")
			So(received.Params[1], ShouldEqual, "{0.25,0.75}")
			So(received.Params[2], ShouldEqual, `{"a":1}`)
		})

		Convey("returns error when metadata cannot marshal", func() {
			err := store.InsertDocument(context.Background(), "x", nil, make(chan int))
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "metadata json")
		})
	})
}

func TestStore_HybridSearch(t *testing.T) {
	Convey("Store.HybridSearch", t, func() {
		var received TableQueryBody

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			bodyBytes, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(bodyBytes, &received)
			testRespondJSON(w, http.StatusOK, []byte(`{}`))
		}))
		defer srv.Close()

		client, err := NewClient(Config{
			BaseURL:   srv.URL,
			APIKey:    "k",
			OrgID:     "o",
			Workspace: "w",
		})
		So(err, ShouldBeNil)

		store := NewStore(client)

		Convey("rejects non-positive limit", func() {
			resp, err := store.HybridSearch(context.Background(), []float32{1}, "q", 0, 0.5, 0.5)
			So(err, ShouldNotBeNil)
			So(resp, ShouldBeNil)
			So(err.Error(), ShouldContainSubstring, "limit")
		})

		Convey("uses default hybrid weights when both are zero", func() {
			_, err := store.HybridSearch(context.Background(), []float32{0.1, 0.2}, "find me", 10, 0, 0)
			So(err, ShouldBeNil)
			So(received.Query, ShouldContainSubstring, `LIMIT 10`)
			So(received.Query, ShouldContainSubstring, `deeplake_hybrid_record`)
			So(received.Query, ShouldContainSubstring, "0.7")
			So(received.Query, ShouldContainSubstring, "0.3")
			So(strings.Contains(received.Query, `"w"."knowledge_base"`), ShouldBeTrue)
			So(len(received.Params), ShouldEqual, 2)
			So(received.Params[0], ShouldEqual, "{0.1,0.2}")
			So(received.Params[1], ShouldEqual, "find me")
		})

		Convey("uses provided weights when non-zero", func() {
			_, err := store.HybridSearch(context.Background(), []float32{1}, "q", 3, 0.6, 0.4)
			So(err, ShouldBeNil)
			So(received.Query, ShouldContainSubstring, "0.6")
			So(received.Query, ShouldContainSubstring, "0.4")
			So(received.Query, ShouldContainSubstring, `LIMIT 3`)
		})
	})
}

func BenchmarkFormatFloat4ArrayLiteral_empty(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		_ = FormatFloat4ArrayLiteral(nil)
	}
}

func BenchmarkFormatFloat4ArrayLiteral_medium(b *testing.B) {
	vec := make([]float32, 768)
	for i := range vec {
		vec[i] = float32(i) * 0.001
	}
	b.ResetTimer()
	for b.Loop() {
		_ = FormatFloat4ArrayLiteral(vec)
	}
}
