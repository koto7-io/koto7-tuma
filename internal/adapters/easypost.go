package adapters

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// EasyPostAdapter verifies inbound EasyPost webhooks using the legacy HMAC scheme
// implemented by EasyPost's official client libraries (X-Hmac-Signature).
// v2 (x-hmac-signature-v2) is not in SDKs yet — add when they ship it.
type EasyPostAdapter struct{}

func (EasyPostAdapter) Verify(headers http.Header, body []byte, secret string) bool {
	sig := headers.Get("X-Hmac-Signature")
	if sig == "" || secret == "" {
		return false
	}
	expected := easypostLegacyDigest(secret, body)
	return hmac.Equal([]byte(strings.ToLower(sig)), []byte(strings.ToLower(expected)))
}

func easypostLegacyDigest(secret string, body []byte) string {
	normalized := norm.NFKD.String(secret)
	mac := hmac.New(sha256.New, []byte(normalized))
	mac.Write(body)
	return "hmac-sha256-hex=" + hex.EncodeToString(mac.Sum(nil))
}

func (EasyPostAdapter) ExtractEventID(_ http.Header, body []byte) string {
	var evt struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(body, &evt) == nil && evt.ID != "" {
		return evt.ID
	}
	return hashBody(body)
}
