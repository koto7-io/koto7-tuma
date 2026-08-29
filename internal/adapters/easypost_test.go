package adapters

import (
	"net/http"
	"testing"
)

func TestEasyPostVerifyValid(t *testing.T) {
	a := EasyPostAdapter{}
	secret := "webhook_secret_from_dashboard"
	body := []byte(`{"id":"evt_easypost_test_1","object":"Event","description":"tracker.updated"}`)
	sig := easypostLegacyDigest(secret, body)

	h := http.Header{}
	h.Set("X-Hmac-Signature", sig)

	if !a.Verify(h, body, secret) {
		t.Fatal("expected valid signature")
	}
}

func TestEasyPostVerifyWrongSecret(t *testing.T) {
	a := EasyPostAdapter{}
	body := []byte(`{"id":"evt_1","object":"Event"}`)
	sig := easypostLegacyDigest("correct", body)

	h := http.Header{}
	h.Set("X-Hmac-Signature", sig)

	if a.Verify(h, body, "wrong") {
		t.Fatal("expected invalid signature")
	}
}

func TestEasyPostVerifyMissingHeader(t *testing.T) {
	a := EasyPostAdapter{}
	body := []byte(`{"id":"evt_1"}`)
	if a.Verify(http.Header{}, body, "secret") {
		t.Fatal("expected missing header to fail")
	}
}

func TestEasyPostVerifyHeaderCaseInsensitive(t *testing.T) {
	a := EasyPostAdapter{}
	secret := "s3cr3t"
	body := []byte(`{"id":"evt_2","object":"Event"}`)
	sig := easypostLegacyDigest(secret, body)

	h := http.Header{}
	h.Set("x-hmac-signature", sig)

	if !a.Verify(h, body, secret) {
		t.Fatal("expected lowercase header name to work")
	}
}

func TestEasyPostExtractEventID(t *testing.T) {
	a := EasyPostAdapter{}
	body := []byte(`{"id":"evt_abc","object":"Event"}`)
	got := a.ExtractEventID(http.Header{}, body)
	if got != "evt_abc" {
		t.Fatalf("got %q", got)
	}
}

func TestEasyPostExtractEventIDFallback(t *testing.T) {
	a := EasyPostAdapter{}
	body := []byte(`{"object":"Event","description":"ping"}`)
	got := a.ExtractEventID(http.Header{}, body)
	if got == "" {
		t.Fatal("expected hash fallback")
	}
}
