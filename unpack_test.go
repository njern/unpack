package unpack

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
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
	{file: "testdata/hello.txt", encoding: "gzip", code: http.StatusUnsupportedMediaType, content: "Content-Encoding: gzip set but unable to decompress body"},
	{file: "testdata/hello.txt.zz", encoding: "deflate", code: http.StatusOK, content: "hello"},
	{file: "testdata/hello.txt", encoding: "deflate", code: http.StatusUnsupportedMediaType, content: "Content-Encoding: deflate set but unable to decompress body"},
}

type requestBodyWriter struct{}

// returnBodyHandler is a simple HTTP handler which writes the same body
// back to the client which the client sent to the server originally.
func (rbw requestBodyWriter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "unable to read r.Body", http.StatusInternalServerError)
		return
	}

	w.Write(body)
}

func TestUnpack(t *testing.T) {
	for _, ft := range fileTests {
		buf, err := ioutil.ReadFile(ft.file)
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
			t.Fatalf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
		}

		// Check the response body is what we expect.
		if strings.TrimSuffix(rr.Body.String(), "\n") != ft.content {
			t.Fatalf("handler returned unexpected body: got '%v' want '%v'", rr.Body.String(), ft.content)
		}
	}
}
