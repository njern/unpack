package unpack_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/njern/unpack/v2"
)

func FuzzMiddlewareContentEncoding(f *testing.F) {
	f.Add("gzip", []byte(helloText))
	f.Add("deflate", []byte(helloText))
	f.Add("zstd", []byte(helloText))
	f.Add("gzip, deflate", []byte(helloText))
	f.Add("br", []byte(helloText))
	f.Add("", []byte(helloText))

	f.Fuzz(func(t *testing.T, encoding string, payload []byte) {
		if len(payload) > 64*1024 {
			t.Skip()
		}

		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/test", bytes.NewReader(payload))
		if err != nil {
			t.Fatalf("request: %v", err)
		}

		if encoding != "" {
			req.Header.Set("Content-Encoding", encoding)
		}

		rr := httptest.NewRecorder()
		handler := unpack.Middleware(requestBodyWriter{})
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK && rr.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("unexpected status: %d", rr.Code)
		}
	})
}
