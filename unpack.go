package unpack

import (
	"bufio"
	"compress/flate"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zlib"
	"github.com/klauspost/compress/zstd"
)

// Options configures the unpacking middleware.
type Options struct {
	// MaxDecompressedBytes limits the decoded request body size.
	// A zero or negative value disables the limit.
	MaxDecompressedBytes int64
	// StrictUnsupportedEncodings returns 415 if an unsupported encoding is present.
	StrictUnsupportedEncodings bool
}

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
	return MiddlewareWithOptions(next, Options{})
}

// MiddlewareWithOptions handles unpacking of requests with configurable options.
// When StrictUnsupportedEncodings is enabled, any unsupported encoding returns 415.
// MaxDecompressedBytes limits the decoded body size and returns 413 when exceeded.
func MiddlewareWithOptions(next http.Handler, opts Options) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		encodings := parseContentEncodings(r.Header.Values("Content-Encoding"))
		if len(encodings) == 0 {
			next.ServeHTTP(w, r)
			return
		}

		if !allSupportedEncodings(encodings) {
			if opts.StrictUnsupportedEncodings {
				unsupported := firstUnsupportedEncoding(encodings)
				_ = r.Body.Close()

				http.Error(w, unsupportedEncodingMessage(unsupported), http.StatusUnsupportedMediaType)

				return
			}

			next.ServeHTTP(w, r)

			return
		}

		if !hasDecodableEncodings(encodings) {
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
				buffered := bufio.NewReader(rc)
				if looksLikeZlibHeader(buffered) {
					rc, err = zlib.NewReader(buffered)
				} else {
					rc = flate.NewReader(buffered)
					err = nil
				}
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

		var body io.ReadCloser = wrappedBody
		if opts.MaxDecompressedBytes > 0 {
			body = &maxBytesReadCloser{
				rc:  body,
				max: opts.MaxDecompressedBytes,
			}
		}

		r.Body = body
		r.Header.Del("Content-Encoding")
		r.ContentLength = -1
		r.Header.Del("Content-Length")

		defer func() {
			_ = body.Close()
		}()

		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}

type multiReadCloser struct {
	reader  io.Reader
	closers []io.Closer
}

// io.Reader passthrough should preserve upstream errors.
//
//nolint:wrapcheck
func (m *multiReadCloser) Read(p []byte) (int, error) {
	return m.reader.Read(p)
}

func (m *multiReadCloser) Close() error {
	return closeAll(m.closers)
}

type maxBytesReadCloser struct {
	rc   io.ReadCloser
	max  int64
	read int64
}

// io.ReadCloser passthrough should preserve upstream errors.
//
//nolint:wrapcheck
func (m *maxBytesReadCloser) Read(p []byte) (int, error) {
	if m.max <= 0 {
		return m.rc.Read(p)
	}

	if m.read >= m.max {
		return 0, &http.MaxBytesError{Limit: m.max}
	}

	if int64(len(p)) > m.max-m.read {
		p = p[:m.max-m.read]
	}

	n, err := m.rc.Read(p)

	m.read += int64(n)
	if m.read >= m.max && err == nil {
		return n, &http.MaxBytesError{Limit: m.max}
	}

	return n, err
}

// io.Closer passthrough should preserve upstream errors.
//
//nolint:wrapcheck
func (m *maxBytesReadCloser) Close() error {
	return m.rc.Close()
}

func closeAll(closers []io.Closer) error {
	var err error
	for i := len(closers) - 1; i >= 0; i-- {
		err = errors.Join(err, closers[i].Close())
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

func unsupportedEncodingMessage(encoding string) string {
	return fmt.Sprintf("Content-Encoding: %s is not supported", encoding)
}

func firstUnsupportedEncoding(encodings []string) string {
	for _, encoding := range encodings {
		switch encoding {
		case encodingGzip, encodingDeflate, encodingZstd, encodingIdentity:
			continue
		default:
			return encoding
		}
	}

	return "unknown"
}

func looksLikeZlibHeader(reader *bufio.Reader) bool {
	const (
		zlibHeaderSize    = 2
		zlibMethodDeflate = 8
		zlibMaxWindow     = 7
		zlibCheckShift    = 8
		zlibCheckMod      = 31
		zlibMethodMask    = 0x0F
	)

	header, err := reader.Peek(zlibHeaderSize)
	if err != nil {
		return true
	}

	cmf := header[0]
	flg := header[1]

	if cmf&zlibMethodMask != zlibMethodDeflate {
		return false
	}

	if cmf>>4 > zlibMaxWindow {
		return false
	}

	value := int(cmf)<<zlibCheckShift + int(flg)

	return value%zlibCheckMod == 0
}
