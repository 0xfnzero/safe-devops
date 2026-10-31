package broker

import (
	"context"
	"crypto/ed25519"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"safeops/internal/core"
	"safeops/internal/store"
)

type Broker struct {
	Config     core.Config
	Store      *store.Store
	Key        ed25519.PrivateKey
	Demo       bool
	DemoData   any
	Now        func() time.Time
	policyHash string
	mu         sync.Mutex
	limits     map[string]bucket
}
type bucket struct {
	start time.Time
	count int
}
type apiError struct {
	code    int
	message string
}

func (e apiError) Error() string     { return e.message }
func bad(code int, msg string) error { return apiError{code, msg} }
func New(ctx context.Context, c core.Config, s *store.Store, key ed25519.PrivateKey, demo bool) (*Broker, error) {
	if e := c.Validate(demo); e != nil {
		return nil, e
	}
	if len(key) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid broker key")
	}
	for _, p := range c.Principals {
		if core.PublicKeyID(p.PublicKey) == core.Hash(key.Public().(ed25519.PublicKey)) {
			return nil, errors.New("broker and approver signing keys must be distinct")
		}
	}
	// Policy changes require an explicit maintenance migration, never silently
	// reinterpret pending approvals using new policy.
	hash := PolicyHash(c, key)
	if e := s.Update(ctx, func(st *core.State) error {
		if st.PolicyHash != "" && st.PolicyHash != hash {
			return errors.New("policy changed: drain requests, wait for grants to expire and run policy-apply")
		}
		st.PolicyHash = hash
		return nil
	}); e != nil {
		return nil, e
	}
	return &Broker{Config: c, Store: s, Key: key, Demo: demo, Now: time.Now, policyHash: hash, limits: map[string]bucket{}}, nil
}
func PolicyHash(c core.Config, key ed25519.PrivateKey) string {
	return core.Hash(core.JSON(struct {
		Principals      []core.Principal
		Agents          []core.AgentIdentity
		Targets         []core.Target
		Projects        []core.Project
		RiskLockSeconds int                `json:",omitempty"`
		DevicePolicy    *core.DevicePolicy `json:",omitempty"`
		WorkspaceID     string
		Key             []byte
	}{c.Principals, c.Agents, c.Targets, c.Projects, c.RiskLockSeconds, c.DevicePolicy, c.WorkspaceID, key.Public().(ed25519.PublicKey)}))
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return bad(415, "JSON content type required")
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return bad(400, "invalid JSON body")
	}
	if e := d.Decode(&struct{}{}); e != io.EOF {
		return bad(400, "one JSON object required")
	}
	return nil
}
func token(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(h, "Bearer ")
}
func equalToken(raw, hashed string) bool {
	return raw != "" && subtle.ConstantTimeCompare([]byte(core.Hash([]byte(raw))), []byte(hashed)) == 1
}
func (b *Broker) principal(r *http.Request) (core.Principal, bool) {
	for _, p := range b.Config.Principals {
		if equalToken(token(r), p.TokenHash) {
			return p, true
		}
	}
	return core.Principal{}, false
}
func (b *Broker) agent(r *http.Request) (core.AgentIdentity, bool) {
	for _, a := range b.Config.Agents {
		if b.Demo {
			if equalToken(token(r), a.TokenHash) {
				return a, true
			}
		} else if r.TLS != nil && len(r.TLS.VerifiedChains) > 0 && r.TLS.VerifiedChains[0][0].Subject.CommonName == a.ID {
			return a, true
		}
	}
	return core.AgentIdentity{}, false
}
func (b *Broker) allow(key string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.Now()
	v := b.limits[key]
	if now.Sub(v.start) >= time.Minute {
		v = bucket{start: now}
	}
	v.count++
	b.limits[key] = v
	if len(b.limits) > 4096 {
		for k, x := range b.limits {
			if now.Sub(x.start) >= time.Minute {
				delete(b.limits, k)
			}
		}
	}
	return v.count <= 240
}
func (b *Broker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	if r.URL.Path == "/healthz" && r.Method == "GET" {
		write(w, 200, map[string]bool{"ok": true})
		return
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if !b.allow(host) {
		write(w, 429, map[string]string{"error": "rate limit exceeded"})
		return
	}
	if b.Demo && r.URL.Path == "/api/demo" && r.Method == "GET" {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			write(w, 403, map[string]string{"error": "demo is loopback only"})
			return
		}
		write(w, 200, b.DemoData)
		return
	}
	var result any
	var err error
	if strings.HasPrefix(r.URL.Path, "/api/agent/") {
		a, ok := b.agent(r)
		if !ok {
			write(w, 401, map[string]string{"error": "agent authentication required"})
			return
		}
		result, err = b.agentRoute(w, r, a)
	} else {
		p, ok := b.principal(r)
		if !ok {
			write(w, 401, map[string]string{"error": "authentication required"})
			return
		}
		if core.Kind(p.Kind) == "human" {
			err = b.verifyDevice(w, r, p.ID)
		}
		if err == nil {
			result, err = b.humanRoute(w, r, p)
		}
	}
	if err != nil {
		var ae apiError
		if errors.As(err, &ae) {
			write(w, ae.code, map[string]string{"error": ae.message})
		} else {
			log.Printf("request failed: %v", err)
			write(w, 503, map[string]string{"error": "service unavailable; execution denied"})
		}
		return
	}
	write(w, 200, result)
}
func (b *Broker) transaction(ctx context.Context, fn func(*core.State) error) error {
	return b.Store.Update(ctx, func(st *core.State) error {
		if st.PolicyHash != b.policyHash {
			return errors.New("policy changed; restart with the applied configuration")
		}
		b.expire(st)
		return fn(st)
	})
}
func (b *Broker) expire(st *core.State) {
	now := b.Now()
	for i := range st.Requests {
		r := &st.Requests[i]
		if r.Status == "pending" && r.Review.Mode == "ai" && !r.NeedsHuman && now.Unix()-r.Intent.CreatedAt >= 45 {
			r.NeedsHuman = true
			r.Assessments = append(r.Assessments, core.Assessment{Reviewer: "system", Decision: "escalate", Reason: "AI review timed out; handed to independent human reviewers.", Model: "system", At: now.Unix(), ContextDigest: r.Context.Digest()})
			store.Append(st, now, "system", "review.timeout", r.Intent.ID, "human handoff")
		}
		if (r.Status == "pending" || r.Status == "approved" || r.Status == "ready") && now.Unix() >= r.Intent.ExpiresAt {
			r.Status = "expired"
			store.Append(st, now, "system", "request.expired", r.Intent.ID, "")
		}
		if r.Status == "running" && now.Unix() >= r.LeaseExpires {
			r.Status = "expired"
			if len(r.Executions) > 0 {
				r.Executions[len(r.Executions)-1].Status = "expired"
			}
			store.Append(st, now, "system", "lease.expired", r.Intent.ID, "execution outcome unknown; no automatic retry")
		}
	}
}
func find(st *core.State, id string) (*core.Request, error) {
	for i := range st.Requests {
		if st.Requests[i].Intent.ID == id {
			return &st.Requests[i], nil
		}
	}
	return nil, bad(404, "request not found")
}
func visible(c core.Config, p core.Principal, r core.Request) bool {
	if core.HasRole(p, "auditor") || r.Intent.Requester == p.ID {
		return true
	}
	_, a, ok := c.Action(r.Intent.Target, r.Intent.Action)
	return ok && slices.Contains(a.Approvers, p.ID)
}
func (b *Broker) humanRoute(w http.ResponseWriter, r *http.Request, p core.Principal) (any, error) {
	if core.Kind(p.Kind) == "ai" && !(r.Method == "GET" && (r.URL.Path == "/api/me" || r.URL.Path == "/api/reviewer/queue" || r.URL.Path == "/api/reviewer/risk-queue")) && !(r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/api/requests/") && strings.HasSuffix(r.URL.Path, "/review")) && !(r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/api/reviewer/risk-scans/")) {
		return nil, bad(403, "AI identity may only read review queue and submit assessments")
	}

	switch {
	case r.URL.Path == "/api/risk-scans" && r.Method == "GET":
		return b.riskHistory(r, p)
	case r.URL.Path == "/api/reviewer/risk-queue" && r.Method == "GET":
		return b.riskQueue(r, p)
	case strings.HasPrefix(r.URL.Path, "/api/reviewer/risk-scans/") && r.Method == "POST":
		return b.riskDecision(w, r, p)
	case strings.HasPrefix(r.URL.Path, "/api/account-locks"):
		return b.accountLocks(w, r, p)
	case r.URL.Path == "/api/terminal/command" && r.Method == "POST":
		return b.terminalCommand(w, r, p)
	case r.URL.Path == "/api/me" && r.Method == "GET":
		p.TokenHash = ""
		return map[string]any{"principal": p, "demo": b.Demo, "workspace": map[string]string{"id": b.Config.WorkspaceID, "name": b.Config.WorkspaceName}}, nil
	case r.URL.Path == "/api/team" && r.Method == "GET":
		members := []core.Principal{}
		for _, member := range b.Config.Principals {
			member.TokenHash = ""
			members = append(members, member)
		}
		return map[string]any{"members": members, "workspace": map[string]string{"id": b.Config.WorkspaceID, "name": b.Config.WorkspaceName}}, nil
	case r.URL.Path == "/api/projects" && r.Method == "GET":
		projects := []core.Project{}
		for _, project := range b.Config.Projects {
			if core.HasRole(p, "auditor") || slices.Contains(project.Members, p.ID) {
				projects = append(projects, project)
			}
		}
		return map[string]any{"projects": projects}, nil
	case r.URL.Path == "/api/catalog" && r.Method == "GET":
		targets := []core.Target{}
		for _, t := range b.Config.Targets {
			allowed := slices.Contains(t.Requesters, p.ID) || core.HasRole(p, "auditor")
			for _, a := range t.Actions {
				if slices.Contains(a.Approvers, p.ID) {
					allowed = true
				}
			}
			if allowed {
				t.Actions = append([]core.Action{}, t.Actions...)
				for n := range t.Actions {
					t.Actions[n].Review = t.Actions[n].Review.Effective()
				}
				targets = append(targets, t)
			}
		}
		heartbeats := map[string]int64{}
		e := b.transaction(r.Context(), func(st *core.State) error {
			for _, a := range b.Config.Agents {
				for _, t := range targets {
					if t.ID == a.Target {
						heartbeats[a.Target] = st.Heartbeats[a.ID]
					}
				}
			}
			return nil
		})
		return map[string]any{"targets": targets, "heartbeats": heartbeats}, e
	case r.URL.Path == "/api/audit" && r.Method == "GET":
		if !core.HasRole(p, "auditor") {
			return nil, bad(403, "auditor role required")
		}
		events := []core.Audit{}
		e := b.transaction(r.Context(), func(st *core.State) error {
			start := max(0, len(st.Audit)-200)
			events = append(events, st.Audit[start:]...)
			return nil
		})
		return map[string]any{"events": events}, e
	case r.URL.Path == "/api/requests" && r.Method == "GET":
		requests := []core.Request{}
		e := b.transaction(r.Context(), func(st *core.State) error {
			for i := len(st.Requests) - 1; i >= 0 && len(requests) < 200; i-- {
				if visible(b.Config, p, st.Requests[i]) {
					requests = append(requests, st.Requests[i])
				}
			}
			return nil
		})
		return map[string]any{"requests": requests}, e
	case r.URL.Path == "/api/reviewer/queue" && r.Method == "GET":
		return b.reviewQueue(r, p)
	case r.URL.Path == "/api/requests" && r.Method == "POST":
		return b.createRequest(w, r, p)
	case strings.HasPrefix(r.URL.Path, "/api/requests/"):
		return b.requestAction(w, r, p)
	}
	return nil, bad(404, "not found")
}
func (b *Broker) agentRoute(w http.ResponseWriter, r *http.Request, a core.AgentIdentity) (any, error) {
	if r.Method != "POST" {
		return nil, bad(405, "POST required")
	}
	if r.URL.Path == "/api/agent/poll" {
		var out *core.Envelope
		e := b.transaction(r.Context(), func(st *core.State) error {
			st.Heartbeats[a.ID] = b.Now().Unix()
			for _, req := range st.Requests {
				if req.AgentID == a.ID && req.Status == "running" {
					return nil
				}
			}
			for i := range st.Requests {
				req := &st.Requests[i]
				if locked(st, req.Intent.Requester, b.Now().Unix()) || req.Status != "approved" || !req.Queued || req.Intent.Target != a.Target || req.Used >= core.Uses(req.Intent) {
					continue
				}
				_, action, _ := b.Config.Action(a.Target, req.Intent.Action)
				if !b.satisfied(*req, action) {
					continue
				}
				if b.runCount(st, req.Intent, b.Now().Unix()) >= action.Review.Effective().RateLimit {
					continue
				}
				req.Status = "running"
				req.Queued = false
				req.Used++
				req.Output = ""
				req.ExitCode = nil
				req.FinishedAt = 0
				req.AgentID = a.ID
				req.LeaseID = core.ID()
				req.LeaseExpires = min(req.Intent.ExpiresAt, b.Now().Unix()+int64(action.RunbookSpec().ExecutionTimeout(req.Intent))+5)
				lease := core.Lease{Version: 2, Use: req.Used, ID: req.LeaseID, AgentID: a.ID, Payload: req.Payload, Approvals: req.Approvals, ExpiresAt: req.LeaseExpires}
				if req.Intent.Version == 4 {
					lease.Version = 3
					lease.Operation = req.Operation
				}
				req.Executions = append(req.Executions, core.Execution{Operation: req.Operation, Use: req.Used, LeaseID: req.LeaseID, Status: "running", StartedAt: b.Now().Unix()})
				out = &core.Envelope{Lease: lease, Signature: core.Sign(b.Key, core.LeaseDomain, core.Encode(lease))}
				store.Append(st, b.Now(), a.ID, "lease.issued", req.Intent.ID, req.LeaseID)
				break
			}
			return nil
		})
		return map[string]any{"job": out}, e
	}
	var in struct {
		RequestID string `json:"request_id"`
		LeaseID   string `json:"lease_id"`
		Output    string `json:"output,omitempty"`
		ExitCode  int    `json:"exit_code"`
	}
	if e := decode(w, r, &in); e != nil {
		return nil, e
	}
	var out any
	e := b.transaction(r.Context(), func(st *core.State) error {
		req, e := find(st, in.RequestID)
		if e != nil {
			return e
		}
		if req.AgentID != a.ID || req.Intent.Target != a.Target || in.LeaseID == "" || req.LeaseID != in.LeaseID {
			return bad(403, "lease does not belong to agent")
		}
		switch r.URL.Path {
		case "/api/agent/check":
			out = map[string]bool{"active": req.Status == "running" && !locked(st, req.Intent.Requester, b.Now().Unix())}
			return nil
		case "/api/agent/result":
			if req.Status != "running" && req.Status != "revoked" && req.Status != "expired" {
				return bad(409, "result already recorded")
			}
			if req.FinishedAt != 0 {
				return bad(409, "result already recorded")
			}
			if len(in.Output) > core.MaxOutput || in.ExitCode < -1 || in.ExitCode > 255 {
				return bad(400, "invalid execution result")
			}
			req.Output = in.Output
			req.ExitCode = &in.ExitCode
			req.FinishedAt = b.Now().Unix()
			if req.Status == "running" {
				req.Status = "completed"
				if req.Used < core.Uses(req.Intent) {
					req.Status = "ready"
				}
				if in.ExitCode != 0 && (req.Intent.Kind != "session" || req.Used >= core.Uses(req.Intent)) {
					req.Status = "failed"
				}
			}
			if len(req.Executions) > 0 {
				ex := &req.Executions[len(req.Executions)-1]
				ex.Status = "completed"
				if in.ExitCode != 0 {
					ex.Status = "failed"
				}
				if req.Status == "revoked" || req.Status == "expired" {
					ex.Status = req.Status
				}
				ex.Output = in.Output
				ex.ExitCode = &in.ExitCode
				ex.FinishedAt = b.Now().Unix()
			}
			store.Append(st, b.Now(), a.ID, "execution.result", req.Intent.ID, core.Hash([]byte(in.Output)))
			b.scheduleRiskScan(st, *req)
			out = map[string]bool{"ok": true}
			return nil
		default:
			return bad(404, "not found")
		}
	})
	return out, e
}
