# unpack
Go HTTP middleware which unpacks gzip, deflate, or zstd-encoded HTTP requests from clients.

[![Go Report Card](https://goreportcard.com/badge/github.com/njern/unpack)](https://goreportcard.com/report/github.com/njern/unpack)
[![GoDoc](https://godoc.org/github.com/njern/unpack?status.svg)](https://godoc.org/github.com/njern/unpack)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## Example usage (go-chi)

```go
r := chi.NewRouter()
r.Use(unpack.Middleware)
r.Post("/v1/status", someStatusHandler)
http.ListenAndServe("127.0.0.1:8080", r)
```
