package httputil

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestLimitedReadAll_UnderLimit(t *testing.T) {
	data, err := LimitedReadAll(strings.NewReader("hello"), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("got %q, want %q", string(data), "hello")
	}
}

func TestLimitedReadAll_ExactLimit(t *testing.T) {
	data, err := LimitedReadAll(strings.NewReader("12345"), 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "12345" {
		t.Fatalf("got %q, want %q", string(data), "12345")
	}
}

func TestLimitedReadAll_OverLimit(t *testing.T) {
	_, err := LimitedReadAll(strings.NewReader("123456"), 5)
	if err == nil {
		t.Fatal("expected error for oversized body, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds 5 bytes") {
		t.Fatalf("error = %v, want message about exceeding 5 bytes", err)
	}
}

func TestCheckContentLength_NoHeader(t *testing.T) {
	resp := &http.Response{ContentLength: -1}
	if err := CheckContentLength(resp, 100); err != nil {
		t.Fatalf("unexpected error for missing Content-Length: %v", err)
	}
}

func TestCheckContentLength_UnderLimit(t *testing.T) {
	resp := &http.Response{ContentLength: 50}
	if err := CheckContentLength(resp, 100); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckContentLength_OverLimit(t *testing.T) {
	resp := &http.Response{ContentLength: 200}
	err := CheckContentLength(resp, 100)
	if err == nil {
		t.Fatal("expected error for oversized Content-Length, got nil")
	}
	if !strings.Contains(err.Error(), "200") || !strings.Contains(err.Error(), "100") {
		t.Fatalf("error = %v, want message with both sizes", err)
	}
}

func TestDecodeJSONLimited_ValidJSON(t *testing.T) {
	var v struct {
		Name string `json:"name"`
	}
	err := DecodeJSONLimited(strings.NewReader(`{"name":"test"}`), 100, &v)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Name != "test" {
		t.Fatalf("got %q, want %q", v.Name, "test")
	}
}

func TestDecodeJSONLimited_OversizedBody(t *testing.T) {
	bigJSON := `{"name":"` + strings.Repeat("x", 200) + `"}`
	var v struct {
		Name string `json:"name"`
	}
	err := DecodeJSONLimited(strings.NewReader(bigJSON), 50, &v)
	if err == nil {
		t.Fatal("expected error for oversized body")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %v, want exceeds message", err)
	}
}

func TestDecodeJSONLimited_ValidPrefixOversized(t *testing.T) {
	// First 20 bytes are valid JSON, but total body is 200+ bytes
	body := `{"a":"b"}` + strings.Repeat(" ", 200)
	var v struct {
		A string `json:"a"`
	}
	err := DecodeJSONLimited(strings.NewReader(body), 50, &v)
	// Body is 209 bytes, exceeds 50 byte limit
	if err == nil {
		t.Fatal("expected error for oversized body")
	}
}

func TestDecodeJSONLimited_TrailingGarbage(t *testing.T) {
	var v struct {
		A string `json:"a"`
	}
	err := DecodeJSONLimited(strings.NewReader(`{"a":"b"}{"c":"d"}`), 100, &v)
	if err == nil {
		t.Fatal("expected error for trailing data")
	}
	if !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("error = %v, want trailing message", err)
	}
}

func TestDecodeJSONLimited_UsesNumber(t *testing.T) {
	var v map[string]any
	err := DecodeJSONLimited(strings.NewReader(`{"id":1234567890123456789}`), 100, &v)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	num, ok := v["id"].(json.Number)
	if !ok {
		t.Fatalf("expected json.Number, got %T", v["id"])
	}
	if num.String() != "1234567890123456789" {
		t.Fatalf("got %q, want exact number string", num.String())
	}
}

func TestFlexString_String(t *testing.T) {
	var s FlexString
	if err := json.Unmarshal([]byte(`"hello"`), &s); err != nil {
		t.Fatal(err)
	}
	if s.String() != "hello" {
		t.Fatalf("got %q", s.String())
	}
}

func TestFlexString_Number(t *testing.T) {
	var s FlexString
	if err := json.Unmarshal([]byte(`1234567890123456789`), &s); err != nil {
		t.Fatal(err)
	}
	if s.String() != "1234567890123456789" {
		t.Fatalf("got %q, want exact number", s.String())
	}
}

func TestFlexString_Null(t *testing.T) {
	var s FlexString
	if err := json.Unmarshal([]byte(`null`), &s); err != nil {
		t.Fatal(err)
	}
	if s.String() != "" {
		t.Fatalf("got %q, want empty", s.String())
	}
}
