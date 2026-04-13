// Package httputil provides HTTP response helpers for ingest collectors.
package httputil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// LimitedReadAll reads up to maxBytes from r.
// If the response contains more than maxBytes, it returns an error
// rather than silently truncating.
func LimitedReadAll(r io.Reader, maxBytes int64) ([]byte, error) {
	lr := io.LimitReader(r, maxBytes+1)
	data, err := io.ReadAll(lr)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("response body exceeds %d bytes", maxBytes)
	}
	return data, nil
}

// CheckContentLength returns an error if the response's Content-Length
// header is present and exceeds maxBytes, allowing early rejection
// without reading the body.
func CheckContentLength(resp *http.Response, maxBytes int64) error {
	if resp.ContentLength > maxBytes {
		return fmt.Errorf("response Content-Length %d exceeds limit of %d bytes", resp.ContentLength, maxBytes)
	}
	return nil
}

// DecodeJSONLimited reads up to maxBytes from r, then decodes exactly one
// JSON value into v using UseNumber() to preserve numeric precision.
// Returns an error if the body exceeds maxBytes or contains trailing
// non-whitespace data after the first JSON value.
func DecodeJSONLimited(r io.Reader, maxBytes int64, v any) error {
	data, err := LimitedReadAll(r, maxBytes)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(v); err != nil {
		return err
	}
	// Reject trailing data: if another token can be decoded, the body is malformed.
	var trailing json.RawMessage
	if dec.Decode(&trailing) != io.EOF {
		return fmt.Errorf("trailing data after JSON value")
	}
	return nil
}

// FlexString unmarshals a JSON string, number, or null into a Go string.
// Use for fields that vary between string and numeric representations
// across API responses (e.g. Weibo status IDs).
type FlexString string

func (s *FlexString) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*s = ""
		return nil
	}
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		*s = FlexString(str)
		return nil
	}
	var num json.Number
	if err := json.Unmarshal(data, &num); err == nil {
		*s = FlexString(num.String())
		return nil
	}
	return fmt.Errorf("FlexString: cannot unmarshal %s", string(data))
}

func (s FlexString) String() string {
	return string(s)
}
