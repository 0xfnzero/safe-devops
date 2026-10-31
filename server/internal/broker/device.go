package broker

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	"safeops/internal/core"
)

func inCIDRs(ip netip.Addr, cidrs []string) bool {
	for _, value := range cidrs {
		p, e := netip.ParsePrefix(value)
		if e == nil && p.Contains(ip.Unmap()) {
			return true
		}
	}
	return false
}
func (b *Broker) allowDevicePrelogin(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	return err == nil && b.allow("device-prelogin:"+host)
}
func (b *Broker) clientIP(r *http.Request) (netip.Addr, error) {
	host, _, e := net.SplitHostPort(r.RemoteAddr)
	if e != nil {
		return netip.Addr{}, bad(403, "client IP unavailable")
	}
	ip, e := netip.ParseAddr(host)
	if e != nil {
		return ip, bad(403, "invalid client IP")
	}
	ip = ip.Unmap()
	policy := b.Config.DevicePolicy
	if policy != nil && inCIDRs(ip, policy.TrustedProxyCIDRs) {
		chain := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
		if len(chain) > 8 {
			return ip, bad(403, "invalid proxy chain")
		}
		found := false
		for n := len(chain) - 1; n >= 0; n-- {
			next, e := netip.ParseAddr(strings.TrimSpace(chain[n]))
			if e != nil {
				return ip, bad(403, "trusted proxy must provide a valid client IP")
			}
			ip = next.Unmap()
			if !inCIDRs(ip, policy.TrustedProxyCIDRs) {
				found = true
				break
			}
		}
		if !found {
			return ip, bad(403, "untrusted client IP not present in proxy chain")
		}
	}
	return ip, nil
}
func (b *Broker) verifyDevice(w http.ResponseWriter, r *http.Request, principal string) error {
	policy := b.Config.DevicePolicy
	if policy == nil || !policy.Required {
		return nil
	}
	id := r.Header.Get("X-Safe-Devops-Device")
	var dev *core.Device
	for n := range policy.Devices {
		d := &policy.Devices[n]
		if d.ID == id && !d.Disabled && (principal == "" || slices.Contains(d.Principals, principal)) {
			dev = d
			break
		}
	}
	if dev == nil {
		return bad(403, "authorized device proof required; use the enrolled desktop client")
	}
	if len(dev.AllowedCIDRs) > 0 {
		ip, e := b.clientIP(r)
		if e != nil {
			return e
		}
		if !inCIDRs(ip, dev.AllowedCIDRs) {
			return bad(403, "device is outside its permitted IP ranges")
		}
	}
	ts := r.Header.Get("X-Safe-Devops-Time")
	at, e := strconv.ParseInt(ts, 10, 64)
	now := b.Now().Unix()
	if e != nil || strconv.FormatInt(at, 10) != ts || at < now-30 || at > now+5 {
		return bad(403, "device proof expired; check machine clock")
	}
	nonce := r.Header.Get("X-Safe-Devops-Nonce")
	raw, e := hex.DecodeString(nonce)
	if e != nil || len(raw) != 16 || hex.EncodeToString(raw) != nonce {
		return bad(403, "invalid device nonce")
	}
	sig, e := base64.StdEncoding.DecodeString(r.Header.Get("X-Safe-Devops-Signature"))
	if e != nil || len(sig) != ed25519.SignatureSize {
		return bad(403, "invalid device signature")
	}
	var body []byte
	if r.Body != nil {
		body, e = io.ReadAll(http.MaxBytesReader(w, r.Body, 32768))
		if e != nil {
			return bad(413, "signed request body too large")
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
	}
	pub, e := core.DevicePublic(dev.PublicKey)
	if e != nil {
		return bad(403, "invalid installed device key")
	}
	message := core.DeviceMessage(id, ts, nonce, r.Method, r.URL.RequestURI(), core.Hash(body), core.Hash([]byte(token(r))))
	if !ed25519.Verify(pub, message, sig) {
		return bad(403, "device signature does not match this identity and exact request")
	}
	return b.transaction(r.Context(), func(st *core.State) error {
		fresh := make([]core.DeviceNonce, 0, len(st.DeviceNonces)+1)
		count := 0
		for _, v := range st.DeviceNonces {
			if v.ExpiresAt < now {
				continue
			}
			if v.Device == id {
				if v.Nonce == nonce {
					return bad(409, "device proof already used")
				}
				count++
			}
			fresh = append(fresh, v)
		}
		if count >= 1024 {
			return bad(429, "device request rate exceeded")
		}
		st.DeviceNonces = append(fresh, core.DeviceNonce{Device: id, Nonce: nonce, ExpiresAt: at + 30})
		return nil
	})
}
