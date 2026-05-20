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

// testRespondJSON writes a Fiber-friendly response from httptest servers (Connection: close).
func testRespondJSON(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Connection", "close")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	_, _ = w.Write(body)
}

func TestNewClient(t *testing.T) {
	Convey("NewClient", t, func() {
		valid := Config{
			BaseURL:   "",
			APIKey:    "k",
			OrgID:     "o",
			Workspace: "w",
		}

		Convey("rejects empty APIKey", func() {
			cfg := valid
			cfg.APIKey = ""
			_, err := NewClient(cfg)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "APIKey")
		})

		Convey("rejects whitespace-only APIKey", func() {
			cfg := valid
			cfg.APIKey = "   "
			_, err := NewClient(cfg)
			So(err, ShouldNotBeNil)
		})

		Convey("rejects empty OrgID", func() {
			cfg := valid
			cfg.OrgID = ""
			_, err := NewClient(cfg)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "OrgID")
		})

		Convey("rejects empty Workspace", func() {
			cfg := valid
			cfg.Workspace = ""
			_, err := NewClient(cfg)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "Workspace")
		})

		Convey("uses default BaseURL when empty", func() {
			client, err := NewClient(valid)
			So(err, ShouldBeNil)
			So(client.BaseURL(), ShouldEqual, defaultBaseURL)
			So(client.Workspace(), ShouldEqual, "w")
		})

		Convey("trims trailing slash from BaseURL", func() {
			cfg := valid
			cfg.BaseURL = "https://example.com/path/"
			client, err := NewClient(cfg)
			So(err, ShouldBeNil)
			So(client.BaseURL(), ShouldEqual, "https://example.com/path")
		})
	})
}

func TestClient_Query(t *testing.T) {
	Convey("Client.Query", t, func() {
		var lastPath string
		var lastAuth string
		var lastOrg string
		var received TableQueryBody

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			lastPath = r.URL.Path
			lastAuth = r.Header.Get("Authorization")
			lastOrg = r.Header.Get("X-Activeloop-Org-Id")
			bodyBytes, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(bodyBytes, &received)

			testRespondJSON(w, http.StatusOK, []byte(`{"rows":[]}`))
		}))
		defer srv.Close()

		client, err := NewClient(Config{
			BaseURL:   srv.URL,
			APIKey:    "secret",
			OrgID:     "org-1",
			Workspace: "my workspace",
		})
		So(err, ShouldBeNil)

		Convey("posts to the workspace tables/query path", func() {
			resp, err := client.Query(context.Background(), "SELECT 1")
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, 200)
			So(lastPath, ShouldEqual, "/workspaces/my workspace/tables/query")
			So(lastAuth, ShouldEqual, "Bearer secret")
			So(lastOrg, ShouldEqual, "org-1")
			So(received.Query, ShouldEqual, "SELECT 1")
			So(received.Params, ShouldBeNil)
		})

		Convey("includes params when provided", func() {
			_, err := client.Query(context.Background(), "SELECT $1", "x", 42)
			So(err, ShouldBeNil)
			So(received.Query, ShouldEqual, "SELECT $1")
			So(len(received.Params), ShouldEqual, 2)
			So(received.Params[0], ShouldEqual, "x")
			// JSON numbers decode as float64
			So(received.Params[1], ShouldEqual, float64(42))
		})

		Convey("returns HTTPError on non-2xx", func() {
			srv.Close()
			srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Connection", "close")
				w.WriteHeader(http.StatusTeapot)
				_, _ = w.Write([]byte("short read"))
			}))
			defer srv.Close()

			erroredClient, err := NewClient(Config{
				BaseURL:   srv.URL,
				APIKey:    "secret",
				OrgID:     "org-1",
				Workspace: "ws",
			})
			So(err, ShouldBeNil)

			resp, qerr := erroredClient.Query(context.Background(), "SELECT 1")
			So(qerr, ShouldNotBeNil)
			So(resp, ShouldNotBeNil)
			So(resp.StatusCode, ShouldEqual, http.StatusTeapot)

			httpErr, ok := qerr.(*HTTPError)
			So(ok, ShouldBeTrue)
			So(httpErr.StatusCode, ShouldEqual, http.StatusTeapot)
			So(string(httpErr.Body), ShouldEqual, "short read")
		})
	})
}

func TestQueryResponse_Decode(t *testing.T) {
	Convey("QueryResponse.Decode", t, func() {
		Convey(" unmarshals JSON", func() {
			resp := &QueryResponse{Body: []byte(`{"a":1,"b":"hi"}`)}
			var v map[string]any
			err := resp.Decode(&v)
			So(err, ShouldBeNil)
			So(v["a"], ShouldEqual, float64(1))
			So(v["b"], ShouldEqual, "hi")
		})

		Convey(" wraps invalid JSON", func() {
			resp := &QueryResponse{Body: []byte(`{`)}
			var v map[string]any
			err := resp.Decode(&v)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "deeplake")
		})
	})
}

func TestHTTPError_Error(t *testing.T) {
	Convey("HTTPError.Error formats status and body", t, func() {
		err := &HTTPError{StatusCode: 500, Body: []byte("  server problem  ")}
		So(strings.Contains(err.Error(), "500"), ShouldBeTrue)
		So(strings.Contains(err.Error(), "server problem"), ShouldBeTrue)
	})
}

func TestTableQueryBody_JSON(t *testing.T) {
	Convey("TableQueryBody JSON shape", t, func() {
		body := TableQueryBody{Query: "q", Params: []any{"p"}}
		encoded, err := json.Marshal(body)
		So(err, ShouldBeNil)
		So(string(encoded), ShouldContainSubstring, `"query":"q"`)
		So(string(encoded), ShouldContainSubstring, `"params"`)

		var decoded TableQueryBody
		So(json.Unmarshal(encoded, &decoded), ShouldBeNil)
		So(decoded.Query, ShouldEqual, "q")
		So(len(decoded.Params), ShouldEqual, 1)
	})
}

func BenchmarkNewClient(b *testing.B) {
	cfg := Config{APIKey: "k", OrgID: "o", Workspace: "w"}
	b.ResetTimer()
	for b.Loop() {
		_, err := NewClient(cfg)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkClient_Query_success(b *testing.B) {
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
	if err != nil {
		b.Fatal(err)
	}

	ctx := context.Background()
	sql := "SELECT text FROM t WHERE id = $1"

	b.ResetTimer()
	for b.Loop() {
		_, err := client.Query(ctx, sql, "id-1")
		if err != nil {
			b.Fatal(err)
		}
	}
}
