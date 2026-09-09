package adapters

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func stripeSig(secret string, body []byte, ts time.Time) string {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d", ts.Unix())
	mac.Write([]byte("."))
	mac.Write(body)
	return fmt.Sprintf("t=%d,v1=%s", ts.Unix(), hex.EncodeToString(mac.Sum(nil)))
}

func TestStripeVerifyValid(t *testing.T) {
	a := StripeAdapter{}
	secret := "whsec_test_from_dashboard"
	body := []byte(`{"id":"evt_1","object":"event","type":"invoice.paid"}`)
	h := http.Header{}
	h.Set("Stripe-Signature", stripeSig(secret, body, time.Now()))
	if !a.Verify(h, body, secret) {
		t.Fatal("expected valid signature")
	}
}

func TestStripeVerifyTrimsSecret(t *testing.T) {
	a := StripeAdapter{}
	secret := "whsec_test_from_dashboard"
	body := []byte(`{"id":"evt_1"}`)
	h := http.Header{}
	h.Set("Stripe-Signature", stripeSig(secret, body, time.Now()))
	if !a.Verify(h, body, "  "+secret+"\n") {
		t.Fatal("expected trimmed secret to verify")
	}
}

func TestStripeVerifyWrongSecret(t *testing.T) {
	a := StripeAdapter{}
	body := []byte(`{"id":"evt_1"}`)
	h := http.Header{}
	h.Set("Stripe-Signature", stripeSig("whsec_correct", body, time.Now()))
	if a.Verify(h, body, "whsec_wrong") {
		t.Fatal("expected invalid signature")
	}
}

func TestStripeVerifyGeneratedSecretDoesNotMatchStripe(t *testing.T) {
	a := StripeAdapter{}
	body := []byte(`{"id":"evt_1"}`)
	h := http.Header{}
	h.Set("Stripe-Signature", stripeSig("whsec_stripe_dashboard", body, time.Now()))
	if a.Verify(h, body, "whsec_"+hex.EncodeToString(make([]byte, 24))) {
		t.Fatal("tuma-generated secret must not verify a stripe-signed payload")
	}
}

func TestStripeVerifyMissingHeader(t *testing.T) {
	a := StripeAdapter{}
	if a.Verify(http.Header{}, []byte(`{}`), "whsec_x") {
		t.Fatal("expected missing header to fail")
	}
}

func TestStripeVerifyTooOld(t *testing.T) {
	a := StripeAdapter{}
	secret := "whsec_x"
	body := []byte(`{"id":"evt_old"}`)
	h := http.Header{}
	h.Set("Stripe-Signature", stripeSig(secret, body, time.Now().Add(-6*time.Minute)))
	if a.Verify(h, body, secret) {
		t.Fatal("expected expired timestamp to fail")
	}
}

func TestStripeVerifyIgnoresV0(t *testing.T) {
	a := StripeAdapter{}
	secret := "whsec_x"
	body := []byte(`{"id":"evt_1"}`)
	ts := time.Now()
	v1 := stripeSig(secret, body, ts)
	h := http.Header{}
	h.Set("Stripe-Signature", v1+",v0=deadbeef")
	if !a.Verify(h, body, secret) {
		t.Fatal("v0 scheme should be ignored")
	}
}

func TestStripeExtractEventID(t *testing.T) {
	a := StripeAdapter{}
	got := a.ExtractEventID(http.Header{}, []byte(`{"id":"evt_abc","object":"event"}`))
	if got != "evt_abc" {
		t.Fatalf("got %q", got)
	}
}
