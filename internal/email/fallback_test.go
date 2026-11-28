package email

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBrevoDefinitiveClientRejectionAllowsFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}))
	defer server.Close()

	sender, err := NewBrevoSender("", "test-key", "sender@example.org", "Test", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	err = sender.SendHTML("person@example.org", "Subject", "<p>Body</p>")
	if err == nil || !CanFallback(err) {
		t.Fatalf("SendHTML() error = %v, want fallback-eligible provider rejection", err)
	}
}

func TestBrevoServerErrorDoesNotAllowFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	sender, err := NewBrevoSender("", "test-key", "sender@example.org", "Test", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	err = sender.SendHTML("person@example.org", "Subject", "<p>Body</p>")
	if err == nil || CanFallback(err) {
		t.Fatalf("SendHTML() error = %v, want ambiguous server error without fallback", err)
	}
}
