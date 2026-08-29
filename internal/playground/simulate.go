package playground

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"
)

type SimulateRequest struct {
	Provider          string
	Count             int
	DuplicateEventID  string
}

type SimulateResult struct {
	Status int
	Body   string
	EventID string
}

func SendEvents(baseURL, inboundPath, secret, provider string, req SimulateRequest) ([]SimulateResult, error) {
	if req.Count < 1 {
		req.Count = 1
	}
	url := strings.TrimSuffix(baseURL, "/") + "/e/" + inboundPath
	out := make([]SimulateResult, 0, req.Count)
	for i := 0; i < req.Count; i++ {
		eventID := req.DuplicateEventID
		if eventID == "" {
			eventID = randomEventID(provider)
		} else if req.Count > 1 {
			eventID = fmt.Sprintf("%s_%d", req.DuplicateEventID, i+1)
		}
		body, headers, err := buildSignedRequest(provider, secret, eventID)
		if err != nil {
			return out, err
		}
		httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return out, err
		}
		for k, v := range headers {
			httpReq.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			return out, err
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		out = append(out, SimulateResult{
			Status:  resp.StatusCode,
			Body:    string(respBody),
			EventID: eventID,
		})
	}
	return out, nil
}

func buildSignedRequest(provider, secret, eventID string) ([]byte, map[string]string, error) {
	switch provider {
	case "stripe":
		return stripePayload(secret, eventID)
	case "github":
		return githubPayload(secret, eventID)
	case "easypost":
		return easypostPayload(secret, eventID)
	case "internal":
		return internalPayload(eventID)
	default:
		return nil, nil, fmt.Errorf("unknown provider: %s", provider)
	}
}

func stripePayload(secret, eventID string) ([]byte, map[string]string, error) {
	evt := map[string]any{
		"id":       eventID,
		"object":   "event",
		"type":     "invoice.payment_failed",
		"livemode": false,
		"created":  time.Now().Unix(),
		"data": map[string]any{
			"object": map[string]any{
				"id":       "in_sim_test",
				"object":   "invoice",
				"amount_due": 4900,
				"currency": "usd",
			},
		},
	}
	body, _ := json.Marshal(evt)
	ts := time.Now().Unix()
	payload := fmt.Sprintf("%d.%s", ts, string(body))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	sig := fmt.Sprintf("t=%d,v1=%s", ts, hex.EncodeToString(mac.Sum(nil)))
	return body, map[string]string{
		"Content-Type":     "application/json",
		"Stripe-Signature": sig,
		"User-Agent":       "Tuma-Playground/1.0",
	}, nil
}

func githubPayload(secret, deliveryID string) ([]byte, map[string]string, error) {
	evt := map[string]any{
		"action": "opened",
		"issue": map[string]any{
			"id":    1,
			"title": "Playground test issue",
		},
		"repository": map[string]any{
			"full_name": "koto7/tuma-playground",
		},
	}
	body, _ := json.Marshal(evt)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return body, map[string]string{
		"Content-Type":        "application/json",
		"X-Hub-Signature-256": sig,
		"X-GitHub-Delivery":   deliveryID,
		"X-GitHub-Event":      "issues",
		"User-Agent":          "Tuma-Playground/1.0",
	}, nil
}

func easypostPayload(secret, eventID string) ([]byte, map[string]string, error) {
	evt := map[string]any{
		"id":          eventID,
		"object":      "Event",
		"description": "tracker.updated",
		"result": map[string]any{
			"id":     "trk_sim_test",
			"object": "Tracker",
			"status": "in_transit",
		},
	}
	body, _ := json.Marshal(evt)
	normalized := norm.NFKD.String(secret)
	mac := hmac.New(sha256.New, []byte(normalized))
	mac.Write(body)
	sig := "hmac-sha256-hex=" + hex.EncodeToString(mac.Sum(nil))
	return body, map[string]string{
		"Content-Type":    "application/json",
		"X-Hmac-Signature": sig,
		"User-Agent":      "Tuma-Playground/1.0",
	}, nil
}

func internalPayload(eventID string) ([]byte, map[string]string, error) {
	evt := map[string]any{
		"id":   eventID,
		"type": "playground.test",
		"ts":   time.Now().Unix(),
	}
	body, _ := json.Marshal(evt)
	return body, map[string]string{
		"Content-Type":    "application/json",
		"X-Tuma-Event-Id": eventID,
		"User-Agent":      "Tuma-Playground/1.0",
	}, nil
}

func randomEventID(provider string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	prefix := "evt"
	switch provider {
	case "github":
		prefix = "gh"
	case "easypost":
		prefix = "evt"
	case "internal":
		prefix = "int"
	}
	return fmt.Sprintf("%s_sim_%s", prefix, hex.EncodeToString(b))
}

func BodyPreview(body []byte) string {
	s := strings.TrimSpace(string(body))
	if len(s) > 120 {
		return s[:120] + "…"
	}
	return s
}
