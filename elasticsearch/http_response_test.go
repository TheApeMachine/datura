package elasticsearch

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestHTTPResponseDecode(t *testing.T) {
	Convey("HTTPResponse.Decode", t, func() {
		Convey("unmarshals JSON into target", func() {
			response := &HTTPResponse{Body: []byte(`{"ok":true,"count":2}`)}

			var parsed struct {
				OK    bool `json:"ok"`
				Count int  `json:"count"`
			}

			err := response.Decode(&parsed)
			So(err, ShouldBeNil)
			So(parsed.OK, ShouldBeTrue)
			So(parsed.Count, ShouldEqual, 2)
		})

		Convey("wraps invalid JSON", func() {
			response := &HTTPResponse{Body: []byte("{")}

			var parsed map[string]any
			err := response.Decode(&parsed)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "decode response")
		})
	})
}

func BenchmarkHTTPResponseDecode(b *testing.B) {
	response := &HTTPResponse{Body: []byte(`{"hits":{"hits":[{"_id":"doc-1","_source":{"text":"hello","embedding":[0.1,0.2]}}]}}`)}

	var parsed struct {
		Hits struct {
			Hits []struct {
				ID string `json:"_id"`
			} `json:"hits"`
		} `json:"hits"`
	}

	b.ResetTimer()

	for b.Loop() {
		parsed = struct {
			Hits struct {
				Hits []struct {
					ID string `json:"_id"`
				} `json:"hits"`
			} `json:"hits"`
		}{}

		if err := response.Decode(&parsed); err != nil {
			b.Fatal(err)
		}
	}
}
