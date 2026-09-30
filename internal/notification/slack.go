package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const slackAPIBase = "https://slack.com/api"

// slackUserID matches a Slack member ID. Incoming webhook URLs are not accepted.
var slackUserID = regexp.MustCompile(`^[UW][A-Z0-9]{8,}$`)

// ValidSlackUserID reports whether s is a Slack member ID (U… or W…).
func ValidSlackUserID(s string) bool {
	return slackUserID.MatchString(s)
}

// SlackClient posts alert text as a direct message via chat.postMessage.
// The API host is fixed. Callers cannot supply a URL.
type SlackClient struct {
	token   string
	baseURL string
	http    *http.Client
}

// NewSlackClient returns a client for the workspace bot token.
// An empty token still constructs; PostDM fails until a token is set.
func NewSlackClient(token string) *SlackClient {
	return &SlackClient{
		token:   token,
		baseURL: slackAPIBase,
		http: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// PostDM sends text to the Slack member identified by userID.
// chat.postMessage opens the DM when the bot has chat:write and im:write.
func (c *SlackClient) PostDM(ctx context.Context, userID, text string) error {
	if c == nil || c.token == "" {
		return errors.New("slack bot token is not configured")
	}
	if !ValidSlackUserID(userID) {
		return fmt.Errorf("invalid slack member id")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("slack message is empty")
	}

	payload, err := json.Marshal(map[string]string{
		"channel": userID,
		"text":    text,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat.postMessage", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("slack post: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("slack read: %w", err)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("slack HTTP %d", resp.StatusCode)
	}

	var out struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("slack decode: %w", err)
	}
	if !out.OK {
		if out.Error == "" {
			out.Error = "unknown"
		}
		return fmt.Errorf("slack: %s", out.Error)
	}
	return nil
}

// DMText joins a rendered subject and body into one Slack message.
func DMText(subject, body string) string {
	subject = strings.TrimSpace(subject)
	body = strings.TrimSpace(body)
	if subject == "" {
		return body
	}
	if body == "" {
		return subject
	}
	return subject + "\n\n" + body
}
