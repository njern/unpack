// Package unpack provides HTTP middleware for decoding compressed request bodies.
//
// Supported encodings are gzip, deflate, and zstd. When using options, you can
// enforce strict handling of unknown encodings or cap decoded body size.
package unpack
