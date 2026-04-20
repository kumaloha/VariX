package compilev2

import (
	"context"
	"strings"
	"testing"

	"github.com/kumaloha/VariX/varix/compile"
)

func TestClientVerifyRejectsNilClient(t *testing.T) {
	var client *Client
	_, err := client.Verify(context.Background(), compile.Bundle{}, compile.Output{})
	if err == nil || !strings.Contains(err.Error(), "verify client is nil") {
		t.Fatalf("Verify() error = %v, want verify client is nil", err)
	}
}

func TestClientVerifyDetailedRejectsNilClient(t *testing.T) {
	var client *Client
	_, err := client.VerifyDetailed(context.Background(), compile.Bundle{}, compile.Output{})
	if err == nil || !strings.Contains(err.Error(), "verify client is nil") {
		t.Fatalf("VerifyDetailed() error = %v, want verify client is nil", err)
	}
}
