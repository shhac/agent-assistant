package worker

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConfirmedRejectionClassification(t *testing.T) {
	for _, tt := range []struct {
		name     string
		status   int
		body     string
		rejected bool
	}{
		{"auth", 401, "secret", true}, {"missing", 404, "", true}, {"timeout", 408, "", false},
		{"structured refusal", 409, `{"code":"operation_rejected"}`, true},
		{"legacy refusal", 409, `{"error":"interrupted workers require explicit resume"}`, true},
		{"idempotency", 409, `{"error":"idempotency key reused with different contents"}`, false},
		{"unknown conflict", 409, `{"error":"something else"}`, false},
		{"malformed conflict", 409, `not JSON`, false}, {"provider", 503, "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			client, _ := New(Config{Endpoint: server.URL, Capabilities: []string{"coordinate"}})
			_, err := client.Send(context.Background(), "run", "op", "message")
			var rejected *RejectionError
			if errors.As(err, &rejected) != tt.rejected || errors.Is(err, ErrUncertain) == tt.rejected {
				t.Fatal("wrong certainty", err)
			}
		})
	}
}
