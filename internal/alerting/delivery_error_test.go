package alerting

import (
	"errors"
	"strings"
	"testing"

	"github.com/koto7/tuma/internal/storage"
)

func TestDeliveryErrorText(t *testing.T) {
	slack := storage.AlertRule{NotificationType: "slack_dm"}
	email := storage.AlertRule{NotificationType: "email"}

	got := deliveryErrorText(slack, errors.New("slack: messages_tab_disabled"))
	if !strings.Contains(got, "blocked this DM") {
		t.Fatalf("slack block: %q", got)
	}
	got = deliveryErrorText(slack, errors.New("slack: invalid_auth"))
	if !strings.Contains(got, "SLACK_BOT_TOKEN") {
		t.Fatalf("slack auth: %q", got)
	}
	got = deliveryErrorText(email, errors.New("smtp auth: 535"))
	if !strings.Contains(got, "SMTP_PASSWORD") {
		t.Fatalf("smtp auth: %q", got)
	}
	got = deliveryErrorText(email, errors.New("smtp host is not configured"))
	if !strings.Contains(got, "SMTP_HOST") {
		t.Fatalf("smtp missing: %q", got)
	}
}
