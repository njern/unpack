package unpack

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zlib"
	"github.com/klauspost/compress/zstd"
)

// Middleware which handles unpacking of requests. It supports unpacking
// Content-Encoding: gzip and Content-Encoding: deflate. Other encodings
// are ignored and passed on to the next handler.
// If the client specifies a supported Content-Encoding but this function
// fails to parse the body as such, it will fail the request with
// HTTP 415 and a text/plain error.
func Middleware(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		var err error

		rc := r.Body
		switch encoding := strings.ToLower(r.Header.Get("Content-Encoding")); encoding {
		case "gzip":
			rc, err = gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, fmt.Sprintf("Content-Encoding: %s set but unable to decompress body", encoding), http.StatusUnsupportedMediaType)
				return
			}

			r.Header.Set("Content-Encoding", "identity")

		case "deflate":
			rc, err = zlib.NewReader(r.Body)
			if err != nil {
				http.Error(w, fmt.Sprintf("Content-Encoding: %s set but unable to decompress body", encoding), http.StatusUnsupportedMediaType)
				return
			}

			r.Header.Set("Content-Encoding", "identity")

		case "zstd":
			dec, err := zstd.NewReader(r.Body)
			if err != nil {
				http.Error(w, fmt.Sprintf("Content-Encoding: %s set but unable to decompress body", encoding), http.StatusUnsupportedMediaType)
				return
			}

			rc = dec.IOReadCloser()

			r.Header.Set("Content-Encoding", "identity")
		}

		r.Body = rc
		next.ServeHTTP(w, r)

		rc.Close() // Make sure we close the reader.
	}

	return http.HandlerFunc(fn)
}
