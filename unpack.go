// Package unpack provides HTTP middleware for decoding compressed request bodies.
package unpack

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zlib"
	"github.com/klauspost/compress/zstd"
)

const (
	encodingGzip     = "gzip"
	encodingDeflate  = "deflate"
	encodingZstd     = "zstd"
	encodingIdentity = "identity"
)

// Middleware which handles unpacking of requests. It supports unpacking
// Content-Encoding: gzip, deflate, and zstd. Other encodings are ignored
// and passed on to the next handler. If multiple encodings are present,
// all must be supported for unpacking to occur.
// If the client specifies a supported Content-Encoding but this function
// fails to parse the body as such, it will fail the request with
// HTTP 415 and a text/plain error.
func Middleware(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		encodings := parseContentEncodings(r.Header.Values("Content-Encoding"))
		if len(encodings) == 0 || !allSupportedEncodings(encodings) || !hasDecodableEncodings(encodings) {
			next.ServeHTTP(w, r)
			return
		}

		rc := r.Body
		closers := []io.Closer{r.Body}

		for i := len(encodings) - 1; i >= 0; i-- {
			encoding := encodings[i]
			if encoding == encodingIdentity {
				continue
			}

			var err error

			switch encoding {
			case encodingGzip:
				rc, err = gzip.NewReader(rc)
			case encodingDeflate:
				rc, err = zlib.NewReader(rc)
			case encodingZstd:
				var dec *zstd.Decoder

				dec, err = zstd.NewReader(rc)
				if err == nil {
					rc = dec.IOReadCloser()
				}
			}

			if err != nil {
				_ = closeAll(closers)

				http.Error(w, decompressionErrorMessage(encoding), http.StatusUnsupportedMediaType)

				return
			}

			rc = &errorWrappingReadCloser{
				rc:       rc,
				encoding: encoding,
			}
			closers = append(closers, rc)
		}

		wrappedBody := &multiReadCloser{
			reader:  rc,
			closers: closers,
		}
		r.Body = wrappedBody
		r.Header.Del("Content-Encoding")
		r.ContentLength = -1
		r.Header.Del("Content-Length")

		defer func() {
			_ = wrappedBody.Close()
		}()

		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}

// DecompressionError is returned when a supported Content-Encoding fails to decode.
type DecompressionError struct {
	Encoding string
	Err      error
}

func (e *DecompressionError) Error() string {
	if e == nil {
		return "Content-Encoding decode error"
	}

	if e.Err == nil {
		return decompressionErrorMessage(e.Encoding)
	}

	return fmt.Sprintf("%s: %v", decompressionErrorMessage(e.Encoding), e.Err)
}

func (e *DecompressionError) Unwrap() error {
	if e == nil {
		return nil
	}

	return e.Err
}

type errorWrappingReadCloser struct {
	rc       io.ReadCloser
	encoding string
}

//nolint:wrapcheck // io.ReadCloser passthrough should preserve upstream errors.
func (r *errorWrappingReadCloser) Read(p []byte) (int, error) {
	n, err := r.rc.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		var decErr *DecompressionError
		if errors.As(err, &decErr) {
			return n, err
		}

		return n, &DecompressionError{Encoding: r.encoding, Err: err}
	}

	return n, err
}

//nolint:wrapcheck // io.Closer passthrough should preserve upstream errors.
func (r *errorWrappingReadCloser) Close() error {
	return r.rc.Close()
}

type multiReadCloser struct {
	reader  io.Reader
	closers []io.Closer
}

//nolint:wrapcheck // io.Reader passthrough should preserve upstream errors.
func (m *multiReadCloser) Read(p []byte) (int, error) {
	return m.reader.Read(p)
}

func (m *multiReadCloser) Close() error {
	return closeAll(m.closers)
}

func closeAll(closers []io.Closer) error {
	var err error
	for i := len(closers) - 1; i >= 0; i-- {
		if cerr := closers[i].Close(); cerr != nil && err == nil {
			err = cerr
		}
	}

	return err
}

func parseContentEncodings(headerValues []string) []string {
	var encodings []string

	for _, value := range headerValues {
		for part := range strings.SplitSeq(value, ",") {
			encoding := strings.ToLower(strings.TrimSpace(part))
			if encoding == "" {
				continue
			}

			encodings = append(encodings, encoding)
		}
	}

	return encodings
}

func allSupportedEncodings(encodings []string) bool {
	for _, encoding := range encodings {
		switch encoding {
		case encodingGzip, encodingDeflate, encodingZstd, encodingIdentity:
			continue
		default:
			return false
		}
	}

	return true
}

func hasDecodableEncodings(encodings []string) bool {
	for _, encoding := range encodings {
		switch encoding {
		case encodingGzip, encodingDeflate, encodingZstd:
			return true
		}
	}

	return false
}

func decompressionErrorMessage(encoding string) string {
	return fmt.Sprintf("Content-Encoding: %s set but unable to decompress body", encoding)
}
