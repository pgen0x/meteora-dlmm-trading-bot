package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Forwarder POSTs HMAC-signed signals to the Hermes webhook endpoint.
// Signing scheme matches gobot's web3_wallet_tracker: hex(HMAC-SHA256(secret, body))
// in the X-Webhook-Signature header.
type Forwarder struct {
	url    string
	secret string
	client *http.Client
}

// New builds a Forwarder for the given Hermes webhook URL + shared secret.
func New(url, secret string) *Forwarder {
	return &Forwarder{url: url, secret: secret, client: &http.Client{Timeout: 10 * time.Second}}
}

// Signal is the webhook envelope Hermes receives. `source` lets the agent's
// subscription prompt distinguish this feed from other webhooks.
type Signal struct {
	Type      string      `json:"type"`
	Timestamp int64       `json:"timestamp"`
	Source    string      `json:"source"`
	Payload   interface{} `json:"payload"`
}

// Send delivers one signal envelope. `nowUnix` is passed in so the caller
// controls the timestamp (keeps this package free of hidden clock reads).
func (f *Forwarder) Send(source string, payload interface{}, nowUnix int64) error {
	sig := Signal{Type: "alert", Timestamp: nowUnix, Source: source, Payload: payload}
	body, err := json.Marshal(sig)
	if err != nil {
		return err
	}

	mac := hmac.New(sha256.New, []byte(f.secret))
	mac.Write(body)
	sum := hex.EncodeToString(mac.Sum(nil))

	req, err := http.NewRequest(http.MethodPost, f.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Signature", sum)

	resp, err := f.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("webhook %d: %s", resp.StatusCode, string(b))
	}
	return nil
}
