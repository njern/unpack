package unpack

import (
	"bytes"
	"fmt" // Added for benchmark naming
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

type fileTest struct {
	file     string
	encoding string
	code     int
	content  string
}

var fileTests = []fileTest{
	{file: "testdata/hello.txt", encoding: "identity", code: http.StatusOK, content: "hello"},
	{file: "testdata/hello.txt.gz", encoding: "gzip", code: http.StatusOK, content: "hello"},
	{file: "testdata/hello.txt.zz", encoding: "deflate", code: http.StatusOK, content: "hello"},
	{file: "testdata/hello.txt.zst", encoding: "zstd", code: http.StatusOK, content: "hello"},
	{file: "testdata/hello.txt", encoding: "gzip", code: http.StatusUnsupportedMediaType, content: "Content-Encoding: gzip set but unable to decompress body"},
	{file: "testdata/hello.txt", encoding: "deflate", code: http.StatusUnsupportedMediaType, content: "Content-Encoding: deflate set but unable to decompress body"},
	{file: "testdata/hello.txt", encoding: "zstd", code: http.StatusInternalServerError, content: "unable to read r.Body"}, // zstd works slightly differently than gzip/deflate.
}

type requestBodyWriter struct{}

// returnBodyHandler is a simple HTTP handler which writes the same body
// back to the client which the client sent to the server originally.
func (rbw requestBodyWriter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "unable to read r.Body", http.StatusInternalServerError)
		return
	}

	_, err = w.Write(body) // Check the error returned by w.Write
	if err != nil {
		http.Error(w, "unable to write response body", http.StatusInternalServerError)
	}
}

func TestUnpack(t *testing.T) {
	for _, ft := range fileTests {
		buf, err := os.ReadFile(ft.file)
		if err != nil {
			t.Fatal(err)
		}

		// Create a request to pass to our requestBodyWriter.
		req, err := http.NewRequest("POST", "/test", bytes.NewBuffer(buf))
		if err != nil {
			t.Fatal(err)
		}

		// Set the Content-Encoding header according to the test
		req.Header.Set("Content-Encoding", ft.encoding)

		// Create a ResponseRecorder (which satisfies http.ResponseWriter) to record the response.
		rr := httptest.NewRecorder()

		// Wrap a requestBodyWriter with our middleware to test it
		handler := Middleware(requestBodyWriter{})

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

// benchmarkFileTests defines the cases suitable for benchmarking (successful operations).
var benchmarkFileTests = []fileTest{
	{file: "testdata/hello.txt", encoding: "identity", code: http.StatusOK, content: "hello"},
	{file: "testdata/hello.txt.gz", encoding: "gzip", code: http.StatusOK, content: "hello"},
	{file: "testdata/hello.txt.zz", encoding: "deflate", code: http.StatusOK, content: "hello"},
	{file: "testdata/hello.txt.zst", encoding: "zstd", code: http.StatusOK, content: "hello"},
}

func BenchmarkUnpack(b *testing.B) {
	for _, ft := range benchmarkFileTests {
		buf, err := os.ReadFile(ft.file)
		if err != nil {
			b.Fatalf("Failed to read file %s for benchmarking: %v", ft.file, err)
		}

		handler := Middleware(requestBodyWriter{})

		benchName := fmt.Sprintf("file_%s_encoding_%s", strings.ReplaceAll(strings.ReplaceAll(ft.file, "testdata/", ""), ".", "_"), ft.encoding)

		b.Run(benchName, func(b *testing.B) {
			b.ResetTimer() // Reset timer to exclude setup like ReadFile and handler creation from this specific sub-benchmark's timing.
			for b.Loop() {
				req, err := http.NewRequest("POST", "/test", bytes.NewBuffer(buf))
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
