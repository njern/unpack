package unpack_test

import (
	"bytes"
	"compress/flate"
	"context"
	"errors"
	"fmt" // Added for benchmark naming
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zlib"
	"github.com/njern/unpack"
)

type fileTest struct {
	file     string
	encoding string
	code     int
	content  string
}

const helloText = "hello"

var fileTests = []fileTest{
	{file: "testdata/hello.txt", encoding: "identity", code: http.StatusOK, content: helloText},
	{file: "testdata/hello.txt.gz", encoding: "gzip", code: http.StatusOK, content: helloText},
	{file: "testdata/hello.txt.zz", encoding: "deflate", code: http.StatusOK, content: helloText},
	{file: "testdata/hello.txt.zst", encoding: "zstd", code: http.StatusOK, content: helloText},
	{file: "testdata/hello.txt", encoding: "gzip", code: http.StatusUnsupportedMediaType, content: "Content-Encoding: gzip set but unable to decompress body"},
	{file: "testdata/hello.txt", encoding: "deflate", code: http.StatusUnsupportedMediaType, content: "Content-Encoding: deflate set but unable to decompress body"},
	{file: "testdata/hello.txt", encoding: "zstd", code: http.StatusUnsupportedMediaType, content: "Content-Encoding: zstd set but unable to decompress body"},
}

type requestBodyWriter struct{}

// returnBodyHandler is a simple HTTP handler which writes the same body
// back to the client which the client sent to the server originally.
func (rbw requestBodyWriter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, maxErr.Error(), http.StatusRequestEntityTooLarge)

			return
		}

		var decErr *unpack.DecompressionError
		if errors.As(err, &decErr) {
			http.Error(w, fmt.Sprintf("Content-Encoding: %s set but unable to decompress body", decErr.Encoding), http.StatusUnsupportedMediaType)

			return
		}

		http.Error(w, "unable to read r.Body", http.StatusInternalServerError)

		return
	}

	_, err = w.Write(body) // Check the error returned by w.Write
	if err != nil {
		http.Error(w, "unable to write response body", http.StatusInternalServerError)
	}
}

func TestUnpack(t *testing.T) {
	t.Parallel()

	for _, ft := range fileTests {
		buf, err := os.ReadFile(ft.file)
		if err != nil {
			t.Fatal(err)
		}

		// Create a request to pass to our requestBodyWriter.
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/test", bytes.NewBuffer(buf))
		if err != nil {
			t.Fatal(err)
		}

		// Set the Content-Encoding header according to the test
		req.Header.Set("Content-Encoding", ft.encoding)

		// Create a ResponseRecorder (which satisfies http.ResponseWriter) to record the response.
		rr := httptest.NewRecorder()

		// Wrap a requestBodyWriter with our middleware to test it
		handler := unpack.Middleware(requestBodyWriter{})

		// Our handlers satisfy http.Handler, so we can call their ServeHTTP method
		// directly and pass in our Request and ResponseRecorder.
		handler.ServeHTTP(rr, req)

		// Check the status code is what we expect.
		if status := rr.Code; status != ft.code {
			t.Fatalf("%#v: handler returned wrong status code: got %v want %v", ft, status, ft.code)
		}

		// Check the response body is what we expect.
		if strings.TrimSuffix(rr.Body.String(), "\n") != ft.content {
			t.Fatalf("%#v: handler returned unexpected body: got '%v' want '%v'", ft, rr.Body.String(), ft.content)
		}
	}
}

func TestUnpackHeaderNormalization(t *testing.T) {
	t.Parallel()

	buf, err := os.ReadFile("testdata/hello.txt.gz")
	if err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/test", bytes.NewBuffer(buf))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Content-Encoding", " GZIP ")
	req.Header.Set("Content-Length", strconv.Itoa(len(buf)))
	req.ContentLength = int64(len(buf))

	rr := httptest.NewRecorder()
	handler := unpack.Middleware(headerCheckHandler{t: t})
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("handler returned wrong status code: got %v want %v", rr.Code, http.StatusOK)
	}
}

func TestUnpackMultipleEncodings(t *testing.T) {
	t.Parallel()

	original := []byte(helloText)
	encoded := deflateData(t, gzipData(t, original))

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/test", bytes.NewBuffer(encoded))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Content-Encoding", "gzip, deflate")

	rr := httptest.NewRecorder()
	handler := unpack.Middleware(requestBodyWriter{})
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("handler returned wrong status code: got %v want %v", rr.Code, http.StatusOK)
	}

	if strings.TrimSuffix(rr.Body.String(), "\n") != helloText {
		t.Fatalf("handler returned unexpected body: got '%v' want '%v'", rr.Body.String(), helloText)
	}
}

func TestUnpackMultiHeaderEncodings(t *testing.T) {
	t.Parallel()

	original := []byte(helloText)
	encoded := deflateData(t, gzipData(t, original))

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/test", bytes.NewBuffer(encoded))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Add("Content-Encoding", "gzip")
	req.Header.Add("Content-Encoding", "deflate")

	rr := httptest.NewRecorder()
	handler := unpack.Middleware(requestBodyWriter{})
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("handler returned wrong status code: got %v want %v", rr.Code, http.StatusOK)
	}

	if strings.TrimSuffix(rr.Body.String(), "\n") != helloText {
		t.Fatalf("handler returned unexpected body: got '%v' want '%v'", rr.Body.String(), helloText)
	}
}

func TestUnpackUnsupportedEncodingPassThrough(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/test", bytes.NewBufferString(helloText))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Content-Encoding", "br")

	rr := httptest.NewRecorder()
	handler := unpack.Middleware(requestBodyWriter{})
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("handler returned wrong status code: got %v want %v", rr.Code, http.StatusOK)
	}

	if strings.TrimSuffix(rr.Body.String(), "\n") != helloText {
		t.Fatalf("handler returned unexpected body: got '%v' want '%v'", rr.Body.String(), helloText)
	}
}

func TestUnpackUnsupportedEncodingStrict(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/test", bytes.NewBufferString(helloText))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Content-Encoding", "br")

	rr := httptest.NewRecorder()
	handler := unpack.MiddlewareWithOptions(requestBodyWriter{}, unpack.Options{
		StrictUnsupportedEncodings: true,
	})
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("handler returned wrong status code: got %v want %v", rr.Code, http.StatusUnsupportedMediaType)
	}
}

func TestUnpackMaxDecompressedBytes(t *testing.T) {
	t.Parallel()

	payload := []byte(helloText)
	encoded := gzipData(t, payload)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/test", bytes.NewBuffer(encoded))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Content-Encoding", "gzip")

	rr := httptest.NewRecorder()
	handler := unpack.MiddlewareWithOptions(requestBodyWriter{}, unpack.Options{
		MaxDecompressedBytes: int64(len(payload) - 1),
	})
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("handler returned wrong status code: got %v want %v", rr.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestUnpackMaxDecompressedBytesExact(t *testing.T) {
	t.Parallel()

	payload := []byte(helloText)
	encoded := gzipData(t, payload)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/test", bytes.NewBuffer(encoded))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Content-Encoding", "gzip")

	rr := httptest.NewRecorder()
	handler := unpack.MiddlewareWithOptions(requestBodyWriter{}, unpack.Options{
		MaxDecompressedBytes: int64(len(payload)),
	})
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("handler returned wrong status code: got %v want %v", rr.Code, http.StatusOK)
	}
}

func TestUnpackMaxDecompressedBytesDisabled(t *testing.T) {
	t.Parallel()

	payload := []byte(helloText)
	encoded := gzipData(t, payload)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/test", bytes.NewBuffer(encoded))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Content-Encoding", "gzip")

	rr := httptest.NewRecorder()
	handler := unpack.MiddlewareWithOptions(requestBodyWriter{}, unpack.Options{
		MaxDecompressedBytes: 0,
	})
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("handler returned wrong status code: got %v want %v", rr.Code, http.StatusOK)
	}
}

func TestUnpackRawDeflate(t *testing.T) {
	t.Parallel()

	payload := []byte(helloText)
	encoded := rawDeflateData(t, payload)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/test", bytes.NewBuffer(encoded))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Content-Encoding", "deflate")

	rr := httptest.NewRecorder()
	handler := unpack.Middleware(requestBodyWriter{})
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("handler returned wrong status code: got %v want %v", rr.Code, http.StatusOK)
	}

	if strings.TrimSuffix(rr.Body.String(), "\n") != helloText {
		t.Fatalf("handler returned unexpected body: got '%v' want '%v'", rr.Body.String(), helloText)
	}
}

type headerCheckHandler struct {
	t *testing.T
}

func (h headerCheckHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.t.Helper()

	if got := r.Header.Get("Content-Encoding"); got != "" {
		h.t.Fatalf("Content-Encoding should be cleared after decoding, got %q", got)
	}

	if got := r.Header.Get("Content-Length"); got != "" {
		h.t.Fatalf("Content-Length header should be cleared after decoding, got %q", got)
	}

	if r.ContentLength != -1 {
		h.t.Fatalf("ContentLength should be -1 after decoding, got %d", r.ContentLength)
	}

	w.WriteHeader(http.StatusOK)
}

func gzipData(t *testing.T, payload []byte) []byte {
	t.Helper()

	var buf bytes.Buffer

	writer := gzip.NewWriter(&buf)
	if _, err := writer.Write(payload); err != nil {
		t.Fatal(err)
	}

	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}

func deflateData(t *testing.T, payload []byte) []byte {
	t.Helper()

	var buf bytes.Buffer

	writer := zlib.NewWriter(&buf)
	if _, err := writer.Write(payload); err != nil {
		t.Fatal(err)
	}

	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}

func rawDeflateData(t *testing.T, payload []byte) []byte {
	t.Helper()

	var buf bytes.Buffer

	writer, err := flate.NewWriter(&buf, flate.DefaultCompression)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := writer.Write(payload); err != nil {
		t.Fatal(err)
	}

	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}

// benchmarkFileTests defines the cases suitable for benchmarking (successful operations).
var benchmarkFileTests = []fileTest{
	{file: "testdata/hello.txt", encoding: "identity", code: http.StatusOK, content: helloText},
	{file: "testdata/hello.txt.gz", encoding: "gzip", code: http.StatusOK, content: helloText},
	{file: "testdata/hello.txt.zz", encoding: "deflate", code: http.StatusOK, content: helloText},
	{file: "testdata/hello.txt.zst", encoding: "zstd", code: http.StatusOK, content: helloText},
}

func BenchmarkUnpack(b *testing.B) {
	for _, ft := range benchmarkFileTests {
		buf, err := os.ReadFile(ft.file)
		if err != nil {
			b.Fatalf("Failed to read file %s for benchmarking: %v", ft.file, err)
		}

		handler := unpack.Middleware(requestBodyWriter{})

		benchName := fmt.Sprintf("file_%s_encoding_%s", strings.ReplaceAll(strings.ReplaceAll(ft.file, "testdata/", ""), ".", "_"), ft.encoding)

		b.Run(benchName, func(b *testing.B) {
			b.ResetTimer() // Reset timer to exclude setup like ReadFile and handler creation from this specific sub-benchmark's timing.

			for b.Loop() {
				req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/test", bytes.NewBuffer(buf))
				if err != nil {
					b.Fatal(err)
				}

				req.Header.Set("Content-Encoding", ft.encoding)

				rr := httptest.NewRecorder()
				handler.ServeHTTP(rr, req)

				if rr.Code != ft.code {
					b.Fatalf("Handler returned wrong status code: got %v, want %v. File: %s, Encoding: %s", rr.Code, ft.code, ft.file, ft.encoding)
				}
			}
		})
	}
}
