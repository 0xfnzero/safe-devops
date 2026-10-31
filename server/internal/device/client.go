package device

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"safeops/internal/core"
)

func ReadKey(path string) (ed25519.PrivateKey, error) {
	raw, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	value, e := ssh.ParseRawPrivateKey(raw)
	if e == nil {
		switch key := value.(type) {
		case *ed25519.PrivateKey:
			return *key, nil
		case ed25519.PrivateKey:
			return key, nil
		}
		return nil, errors.New("SSH Ed25519 device private key required")
	}
	// Also supports the existing CLI's 0600 Ed25519 key format.
	return core.ReadKey(path)
}
func Public(key ed25519.PrivateKey) string {
	pub, _ := ssh.NewPublicKey(key.Public())
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub)))
}

type Transport struct {
	Base http.RoundTripper
	Key  ed25519.PrivateKey
	Now  func() time.Time
}

func (t Transport) RoundTrip(r *http.Request) (*http.Response, error) {
	var body []byte
	var e error
	if r.Body != nil {
		body, e = io.ReadAll(io.LimitReader(r.Body, 32769))
		r.Body.Close()
		if e != nil {
			return nil, e
		}
		if len(body) > 32768 {
			return nil, errors.New("device-signed body too large")
		}
	}
	clone := r.Clone(r.Context())
	clone.Header = r.Header.Clone()
	if r.Body != nil {
		clone.Body = io.NopCloser(bytes.NewReader(body))
	}
	now := time.Now()
	if t.Now != nil {
		now = t.Now()
	}
	nonce := make([]byte, 16)
	if _, e = rand.Read(nonce); e != nil {
		return nil, e
	}
	SignHeaders(clone, t.Key, body, strconv.FormatInt(now.Unix(), 10), hex.EncodeToString(nonce))
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(clone)
}
func SignHeaders(r *http.Request, key ed25519.PrivateKey, body []byte, ts, nonce string) {
	id := core.DeviceID(key.Public().(ed25519.PublicKey))
	credential := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	message := core.DeviceMessage(id, ts, nonce, r.Method, r.URL.RequestURI(), core.Hash(body), core.Hash([]byte(credential)))
	r.Header.Set("X-Safe-Devops-Device", id)
	r.Header.Set("X-Safe-Devops-Time", ts)
	r.Header.Set("X-Safe-Devops-Nonce", nonce)
	r.Header.Set("X-Safe-Devops-Signature", base64.StdEncoding.EncodeToString(ed25519.Sign(key, message)))
}
func Enable(client *http.Client, keyFile string) error {
	key, e := ReadKey(keyFile)
	if e != nil {
		return e
	}
	client.Transport = Transport{Base: client.Transport, Key: key}
	return nil
}
