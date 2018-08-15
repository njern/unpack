# unpack
Go HTTP middleware which unpacks gzip or deflate-encoded POST requests from clients 

[![GoDoc Widget]][GoDoc] [![Travis Widget]][Travis]

## Example usage (go-chi)

```go
r := chi.NewRouter()
r.Use(unpack.Middleware)
r.Post("/v1/status", someStatusHandler)
http.ListenAndServe("127.0.0.1:8080", r))
```


[GoDoc]: https://godoc.org/github.com/njern/unpack
[GoDoc Widget]: https://godoc.org/github.com/njern/unpack?status.svg
[Travis]: https://travis-ci.com/njern/unpack
[Travis Widget]: https://travis-ci.com/njern/unpack.svg?branch=master