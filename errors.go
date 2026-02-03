package unpack

import (
	"errors"
	"fmt"
	"io"
)

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

// io.ReadCloser passthrough should preserve upstream errors.
//
//nolint:wrapcheck
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

// io.Closer passthrough should preserve upstream errors.
//
//nolint:wrapcheck
func (r *errorWrappingReadCloser) Close() error {
	return r.rc.Close()
}
