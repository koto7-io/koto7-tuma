package api

import "testing"

func TestValidateAlertRule(t *testing.T) {
	if msg := validateAlertRule("OPEN_ISSUES", "COUNT", 5, "email", "ops@example.com"); msg != "" {
		t.Fatal(msg)
	}
	if msg := validateAlertRule("OPEN_ISSUES", "COUNT", 5, "slack", "https://hooks.slack.com/services/T/B/secret"); msg == "" {
		t.Fatal("slack webhook must be rejected")
	}
	if msg := validateAlertRule("OPEN_ISSUES", "COUNT", 5, "email", "not-an-email"); msg == "" {
		t.Fatal("expected invalid email")
	}
	if msg := validateAlertRule("OPEN_ISSUES", "COUNT", 5, "email", "ops@example.com\r\nBcc: x@y.co"); msg == "" {
		t.Fatal("expected header injection in address to be rejected")
	}
	if msg := validateAlertRule("DELIVERY_SUCCESS", "PERCENT", 101, "email", "ops@example.com"); msg == "" {
		t.Fatal("expected percent cap")
	}
	if msg := validateAlertRule("OPEN_ISSUES", "HOURS", 1, "email", "ops@example.com"); msg == "" {
		t.Fatal("expected unit mismatch")
	}
}
