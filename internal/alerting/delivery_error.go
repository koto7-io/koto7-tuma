package alerting

import (
	"strings"

	"github.com/koto7/tuma/internal/storage"
)

// deliveryErrorText is what the UI shows after a failed send. It says what to
// change. The raw transport error stays in the worker log.
func deliveryErrorText(rule storage.AlertRule, err error) string {
	raw := ""
	if err != nil {
		raw = err.Error()
	}
	switch rule.NotificationType {
	case "slack_dm":
		return slackDeliveryError(raw)
	case "email":
		return emailDeliveryError(raw)
	default:
		return clip(raw, 200)
	}
}

func slackDeliveryError(raw string) string {
	switch {
	case strings.Contains(raw, "not configured"):
		return "Set SLACK_BOT_TOKEN on the Tuma server to send Slack DMs."
	case strings.Contains(raw, "invalid_auth"),
		strings.Contains(raw, "token_revoked"),
		strings.Contains(raw, "not_authed"),
		strings.Contains(raw, "account_inactive"):
		return "Slack rejected the bot token. Check SLACK_BOT_TOKEN and reinstall the app if it was revoked."
	case strings.Contains(raw, "missing_scope"):
		return "The Slack app is missing chat:write or im:write. Add those scopes and reinstall the app."
	case strings.Contains(raw, "channel_not_found"),
		strings.Contains(raw, "user_not_found"),
		strings.Contains(raw, "invalid slack member"):
		return "Slack could not find that member. Check the member ID."
	case strings.Contains(raw, "messages_tab_disabled"),
		strings.Contains(raw, "cannot_dm_bot"),
		strings.Contains(raw, "ekm_access_denied"),
		strings.Contains(raw, "access_denied"),
		strings.Contains(raw, "restricted_action"):
		return "Slack blocked this DM. Allow messages from the Tuma bot for that member, or confirm the member ID."
	default:
		if raw == "" {
			return "Slack DM failed."
		}
		return "Slack DM failed: " + clip(raw, 180)
	}
}

func emailDeliveryError(raw string) string {
	switch {
	case strings.Contains(raw, "smtp host is not configured"):
		return "Set SMTP_HOST, SMTP_PORT, SMTP_USERNAME, SMTP_PASSWORD, and SMTP_FROM on the Tuma server to send email."
	case strings.Contains(raw, "smtp auth"):
		return "SMTP login failed. Check SMTP_USERNAME and SMTP_PASSWORD."
	case strings.Contains(raw, "STARTTLS"), strings.Contains(raw, "starttls"):
		return "The SMTP server refused TLS. Check SMTP_HOST and SMTP_PORT."
	default:
		if raw == "" {
			return "Email failed."
		}
		return "Email failed: " + clip(raw, 180)
	}
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
