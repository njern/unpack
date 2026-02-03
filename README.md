# unpack
Go HTTP middleware which unpacks gzip, deflate, or zstd-encoded HTTP requests from clients.

[![Go Report Card](https://goreportcard.com/badge/github.com/njern/unpack)](https://goreportcard.com/report/github.com/njern/unpack)
[![GoDoc](https://godoc.org/github.com/njern/unpack?status.svg)](https://godoc.org/github.com/njern/unpack)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## Behavior

- Unsupported `Content-Encoding` values are passed through by default.
- Enable strict mode to return HTTP 415 for unsupported encodings.
- Set a maximum decoded size to return HTTP 413 when the limit is exceeded.

## Example usage (go-chi)

```go
r := chi.NewRouter()
r.Use(unpack.Middleware)
r.Post("/v1/status", someStatusHandler)
http.ListenAndServe("127.0.0.1:8080", r)
```

## Example usage with options

```go
r := chi.NewRouter()
r.Use(func(next http.Handler) http.Handler {
	return unpack.MiddlewareWithOptions(next, unpack.Options{
		MaxDecompressedBytes:       10 << 20, // 10 MB
		StrictUnsupportedEncodings: true,
	})
})
r.Post("/v1/status", someStatusHandler)
http.ListenAndServe("127.0.0.1:8080", r)
```

## Example client request (zstd)

```go
encoder, err := zstd.NewWriter(nil)
if err != nil {
	log.Fatal(err)
}
defer encoder.Close()

compressed := encoder.EncodeAll([]byte("hello"), nil)
req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:8080/v1/status", bytes.NewReader(compressed))
if err != nil {
	log.Fatal(err)
}
req.Header.Set("Content-Encoding", "zstd")

resp, err := http.DefaultClient.Do(req)
if err != nil {
	log.Fatal(err)
}
defer resp.Body.Close()
```
