package adapters

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type SourceAdapter interface {
	Verify(headers http.Header, body []byte, secret string) bool
	ExtractEventID(headers http.Header, body []byte) string
}

var registry = map[string]SourceAdapter{
	"stripe":       &StripeAdapter{},
	"github":       &GitHubAdapter{},
	"generic_hmac": &GenericHMACAdapter{Header: "X-Signature", Algorithm: "sha256"},
	"internal":     &InternalAdapter{},
}

func Get(sourceType string) (SourceAdapter, error) {
	a, ok := registry[sourceType]
	if !ok {
		return nil, fmt.Errorf("unknown source type: %s", sourceType)
	}
	return a, nil
}

type StripeAdapter struct{}

func (StripeAdapter) Verify(headers http.Header, body []byte, secret string) bool {
	sig := headers.Get("Stripe-Signature")
	if sig == "" || secret == "" {
		return false
	}
	var ts int64
	var v1s []string
	for _, part := range strings.Split(sig, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			ts, _ = strconv.ParseInt(kv[1], 10, 64)
		case "v1":
			v1s = append(v1s, kv[1])
		}
	}
	if ts == 0 || len(v1s) == 0 {
		return false
	}
	if time.Since(time.Unix(ts, 0)) > 5*time.Minute {
		return false
	}
	payload := fmt.Sprintf("%d.%s", ts, string(body))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	expected := hex.EncodeToString(mac.Sum(nil))
	for _, v := range v1s {
		if hmac.Equal([]byte(v), []byte(expected)) {
			return true
		}
	}
	return false
}

func (StripeAdapter) ExtractEventID(headers http.Header, body []byte) string {
	var evt struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(body, &evt) == nil && evt.ID != "" {
		return evt.ID
	}
	return hashBody(body)
}

type GitHubAdapter struct{}

func (GitHubAdapter) Verify(headers http.Header, body []byte, secret string) bool {
	sig := headers.Get("X-Hub-Signature-256")
	if sig == "" || secret == "" {
		return false
	}
	if !strings.HasPrefix(sig, "sha256=") {
		return false
	}
	got := strings.TrimPrefix(sig, "sha256=")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(got), []byte(expected))
}

func (GitHubAdapter) ExtractEventID(headers http.Header, body []byte) string {
	if id := headers.Get("X-GitHub-Delivery"); id != "" {
		return id
	}
	return hashBody(body)
}

type GenericHMACAdapter struct {
	Header    string
	Algorithm string
}

func (g *GenericHMACAdapter) Verify(headers http.Header, body []byte, secret string) bool {
	sig := headers.Get(g.Header)
	if sig == "" || secret == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	// accept raw hex or prefixed
	sig = strings.TrimPrefix(sig, "sha256=")
	return hmac.Equal([]byte(sig), []byte(expected))
}

func (g *GenericHMACAdapter) ExtractEventID(headers http.Header, body []byte) string {
	if id := headers.Get("X-Event-Id"); id != "" {
		return id
	}
	if id := headers.Get("X-Request-Id"); id != "" {
		return id
	}
	return hashBody(body)
}

type InternalAdapter struct{}

func (InternalAdapter) Verify(_ http.Header, _ []byte, _ string) bool {
	return true
}

func (InternalAdapter) ExtractEventID(headers http.Header, body []byte) string {
	if id := headers.Get("X-Tuma-Event-Id"); id != "" {
		return id
	}
	return hashBody(body)
}

func hashBody(body []byte) string {
	h := sha256.Sum256(body)
	return hex.EncodeToString(h[:16])
}
