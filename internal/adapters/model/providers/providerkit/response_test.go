package providerkit

import (
	"io"
	"strings"
	"testing"
)

func TestDecodeJSONResponse(t *testing.T) {
	var payload struct {
		Value string `json:"value"`
	}
	if err := DecodeJSONResponse(strings.NewReader(`{"value":"ok"}`), &payload); err != nil {
		t.Fatalf("DecodeJSONResponse() error = %v", err)
	}
	if payload.Value != "ok" {
		t.Fatalf("payload.Value = %q", payload.Value)
	}
}

func TestDecodeJSONResponseRejectsOversizedBody(t *testing.T) {
	reader := io.LimitReader(zeroReader{}, MaxJSONResponseBytes+1)
	if err := DecodeJSONResponse(reader, &struct{}{}); err == nil {
		t.Fatal("oversized JSON response was accepted")
	}
}

type zeroReader struct{}

func (zeroReader) Read(buffer []byte) (int, error) {
	for index := range buffer {
		buffer[index] = '0'
	}
	return len(buffer), nil
}
