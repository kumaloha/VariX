package httputil

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestValidatePublicURLRejectsPrivateAndMetadataTargets(t *testing.T) {
	for _, raw := range []string{
		"http://127.0.0.1:8000/internal",
		"http://localhost:8000/internal",
		"http://10.0.0.1/internal",
		"http://172.16.0.1/internal",
		"http://192.168.1.1/internal",
		"http://169.254.169.254/latest/meta-data/",
		"http://100.100.100.200/latest/meta-data/",
		"file:///etc/passwd",
	} {
		t.Run(raw, func(t *testing.T) {
			err := ValidatePublicURL(context.Background(), raw)
			if err == nil {
				t.Fatalf("ValidatePublicURL(%q) error = nil, want rejection", raw)
			}
		})
	}
}

func TestPublicHTTPClientRejectsPrivateRedirectTarget(t *testing.T) {
	client := NewPublicHTTPClient(5*time.Second, nil)
	req, err := http.NewRequest(http.MethodGet, "https://example.com/start", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	redirect, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:8000/admin", nil)
	if err != nil {
		t.Fatalf("NewRequest() redirect error = %v", err)
	}

	err = client.CheckRedirect(redirect, []*http.Request{req})
	if err == nil {
		t.Fatal("CheckRedirect() error = nil, want private target rejection")
	}
	if !strings.Contains(err.Error(), "private") && !strings.Contains(err.Error(), "localhost") {
		t.Fatalf("CheckRedirect() error = %q, want private/localhost rejection", err)
	}
}
