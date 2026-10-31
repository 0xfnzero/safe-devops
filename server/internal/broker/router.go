package broker

import (
	"context"
	"errors"
	"net/http"
	"safeops/internal/core"
	"strings"
	"time"
)

type Router struct {
	Brokers []*Broker
	byToken map[string]*Broker
	byAgent map[string]*Broker
	demo    bool
}

func NewRouter(brokers []*Broker, demo bool) (*Router, error) {
	r := &Router{Brokers: brokers, byToken: map[string]*Broker{}, byAgent: map[string]*Broker{}, demo: demo}
	roots := map[string]bool{}
	for _, b := range brokers {
		for _, p := range b.Config.Principals {
			if r.byToken[p.TokenHash] != nil {
				return nil, errors.New("duplicate workspace identity token")
			}
			r.byToken[p.TokenHash] = b
		}
		for _, a := range b.Config.Agents {
			if r.byAgent[a.ID] != nil {
				return nil, errors.New("duplicate workspace agent CN")
			}
			r.byAgent[a.ID] = b
			if demo {
				if r.byToken[a.TokenHash] != nil {
					return nil, errors.New("duplicate agent token")
				}
				r.byToken[a.TokenHash] = b
			}
		}
		if roots[core.Hash(b.Key)] {
			return nil, errors.New("workspace broker keys must be distinct")
		}
		roots[core.Hash(b.Key)] = true
	}
	return r, nil
}
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if req.URL.Path == "/healthz" && req.Method == "GET" {
		write(w, 200, map[string]bool{"ok": true})
		return
	}
	if req.URL.Path == "/readyz" && req.Method == "GET" {
		ctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
		defer cancel()
		for _, b := range r.Brokers {
			if b.Store.Ping(ctx) != nil {
				write(w, 503, map[string]bool{"ok": false})
				return
			}
		}
		write(w, 200, map[string]bool{"ok": true})
		return
	}
	if req.URL.Path == "/api/device/status" && req.Method == "GET" {
		required := false
		for _, b := range r.Brokers {
			if b.Config.DevicePolicy != nil && b.Config.DevicePolicy.Required {
				required = true
			}
		}
		verified := !required
		if required && req.Header.Get("X-Safe-Devops-Device") != "" {
			for _, b := range r.Brokers {
				if b.Config.DevicePolicy == nil || !b.Config.DevicePolicy.Required {
					continue
				}
				if !b.allowDevicePrelogin(req) {
					write(w, 429, map[string]string{"error": "device verification rate exceeded"})
					return
				}
				if e := b.verifyDevice(w, req, ""); e == nil {
					verified = true
					break
				}
			}
		}
		write(w, 200, map[string]bool{"required": required, "verified": verified})
		return
	}
	if req.URL.Path == "/api/device/verify" && req.Method == "POST" {
		id := req.Header.Get("X-Safe-Devops-Device")
		// No token is accepted yet. Device verification confers no user permissions.
		// A machine can be enrolled in several teams; identity binding is checked
		// again for every subsequent authenticated request in its own workspace.
		for _, b := range r.Brokers {
			if b.Config.DevicePolicy == nil || !b.Config.DevicePolicy.Required {
				continue
			}
			for _, d := range b.Config.DevicePolicy.Devices {
				if d.ID != id || d.Disabled {
					continue
				}
				if !b.allowDevicePrelogin(req) {
					write(w, 429, map[string]string{"error": "device verification rate exceeded"})
					return
				}
				if e := b.verifyDevice(w, req, ""); e == nil {
					write(w, 200, map[string]bool{"verified": true})
					return
				}
			}
		}
		write(w, 403, map[string]string{"error": "device is not enrolled, signature is invalid, or IP is not allowed"})
		return
	}
	if r.demo && req.URL.Path == "/api/demo" && len(r.Brokers) == 1 {
		r.Brokers[0].ServeHTTP(w, req)
		return
	}
	var b *Broker
	if strings.HasPrefix(req.URL.Path, "/api/agent/") && !r.demo {
		if req.TLS != nil && len(req.TLS.VerifiedChains) > 0 {
			b = r.byAgent[req.TLS.VerifiedChains[0][0].Subject.CommonName]
		}
	} else if raw := token(req); raw != "" {
		b = r.byToken[core.Hash([]byte(raw))]
	}
	if b == nil {
		write(w, 401, map[string]string{"error": "invalid identity"})
		return
	}
	// No workspace selector in headers, query strings or request bodies is used.
	ctx, cancel := context.WithTimeout(req.Context(), 10*time.Second)
	defer cancel()
	b.ServeHTTP(w, req.WithContext(ctx))
}
