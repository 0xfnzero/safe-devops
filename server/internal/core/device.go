package core

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"net/netip"
	"strings"

	"golang.org/x/crypto/ssh"
)

const DeviceDomain = "safeops.device-request.v1\n"

type DevicePolicy struct {
	Required          bool     `json:"required"`
	Devices           []Device `json:"devices"`
	TrustedProxyCIDRs []string `json:"trusted_proxy_cidrs,omitempty"`
}
type Device struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	PublicKey    string   `json:"ssh_public_key"`
	Principals   []string `json:"principals"`
	AllowedCIDRs []string `json:"allowed_cidrs,omitempty"`
	Disabled     bool     `json:"disabled,omitempty"`
}
type DeviceNonce struct {
	Device    string `json:"device"`
	Nonce     string `json:"nonce"`
	ExpiresAt int64  `json:"expires_at"`
}

func DevicePublic(value string) (ed25519.PublicKey, error) {
	key, _, options, rest, err := ssh.ParseAuthorizedKey([]byte(value))
	if err != nil || len(options) > 0 || len(strings.TrimSpace(string(rest))) > 0 {
		return nil, errors.New("one plain SSH Ed25519 public key required")
	}
	k, ok := key.(ssh.CryptoPublicKey)
	if !ok {
		return nil, errors.New("unsupported device key")
	}
	pub, ok := k.CryptoPublicKey().(ed25519.PublicKey)
	if !ok {
		return nil, errors.New("device keys must be ssh-ed25519")
	}
	return pub, nil
}
func DeviceID(pub ed25519.PublicKey) string { return "device-" + Hash(pub)[:16] }
func DeviceMessage(id, timestamp, nonce, method, path, bodyHash, tokenHash string) []byte {
	return []byte(DeviceDomain + strings.Join([]string{id, timestamp, nonce, method, path, bodyHash, tokenHash}, "\n"))
}
func (p *DevicePolicy) Validate(c Config) error {
	if p == nil {
		return nil
	}
	ids := map[string]bool{}
	keys := map[string]bool{}
	for _, d := range p.Devices {
		pub, e := DevicePublic(d.PublicKey)
		if e != nil || d.ID != DeviceID(pub) || ids[d.ID] || keys[base64.StdEncoding.EncodeToString(pub)] || len(d.Principals) == 0 {
			return errors.New("invalid or duplicate device; ID must match key fingerprint")
		}
		ids[d.ID] = true
		keys[base64.StdEncoding.EncodeToString(pub)] = true
		for _, id := range d.Principals {
			principal, ok := c.Principal(id)
			if !ok || Kind(principal.Kind) != "human" {
				return errors.New("devices must bind existing human principals")
			}
		}
		for _, cidr := range d.AllowedCIDRs {
			if _, e := netip.ParsePrefix(cidr); e != nil {
				return errors.New("device IP restrictions must be explicit CIDRs")
			}
		}
	}
	for _, cidr := range p.TrustedProxyCIDRs {
		prefix, e := netip.ParsePrefix(cidr)
		if e != nil || prefix.Bits() == 0 {
			return errors.New("trusted proxy CIDRs must be explicit, never /0")
		}
	}
	return nil
}
