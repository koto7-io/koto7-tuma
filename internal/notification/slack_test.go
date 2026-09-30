package notification

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSlackClient_PostDM(t *testing.T) {
	var gotAuth, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := NewSlackClient("xoxb-test")
	c.baseURL = srv.URL
	if err := c.PostDM(context.Background(), "U012ABCDEF", "hello"); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer xoxb-test" {
		t.Errorf("auth = %q", gotAuth)
	}
	if gotPath != "/chat.postMessage" {
		t.Errorf("path = %q", gotPath)
	}
	if !strings.Contains(gotBody, `"channel":"U012ABCDEF"`) || !strings.Contains(gotBody, `"text":"hello"`) {
		t.Errorf("body = %s", gotBody)
	}
}

func TestSlackClient_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":false,"error":"channel_not_found"}`))
	}))
	defer srv.Close()

	c := NewSlackClient("xoxb-test")
	c.baseURL = srv.URL
	err := c.PostDM(context.Background(), "U012ABCDEF", "hello")
	if err == nil || !strings.Contains(err.Error(), "channel_not_found") {
		t.Fatalf("err = %v", err)
	}
}

func TestSlackClient_RejectsMissingTokenAndBadID(t *testing.T) {
	c := NewSlackClient("")
	if err := c.PostDM(context.Background(), "U012ABCDEF", "hello"); err == nil {
		t.Fatal("expected missing token error")
	}
	c = NewSlackClient("xoxb-test")
	if err := c.PostDM(context.Background(), "https://hooks.slack.com/services/T/B/secret", "hello"); err == nil {
		t.Fatal("expected webhook URL to be rejected")
	}
}

func TestDMText(t *testing.T) {
	if got := DMText("Subject", "Body"); got != "Subject\n\nBody" {
		t.Fatalf("got %q", got)
	}
}
