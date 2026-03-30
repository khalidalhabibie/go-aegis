package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

const SignatureHeaderName = "X-Aegis-Signature"
const TimestampHeaderName = "X-Aegis-Timestamp"

type Signer struct {
	secret []byte
}

func NewSigner(secret string) *Signer {
	if secret == "" {
		return nil
	}

	return &Signer{secret: []byte(secret)}
}

func (s *Signer) Sign(body []byte, at time.Time) (string, string) {
	if s == nil {
		return "", ""
	}

	timestamp := at.UTC().Format(time.RFC3339)
	signature := ComputeSignature(s.secret, timestamp, body)
	return timestamp, signature
}

func ComputeSignature(secret []byte, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)

	return fmt.Sprintf("v1=%s", hex.EncodeToString(mac.Sum(nil)))
}
