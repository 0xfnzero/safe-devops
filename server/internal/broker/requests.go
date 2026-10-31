package broker

import (
	"net/http"
	"slices"
	"strings"

	"safeops/internal/core"
	"safeops/internal/store"
)

func (b *Broker) satisfied(req core.Request, a core.Action) bool {
	keys, kinds := map[string]string{}, map[string]string{}
	for _, p := range b.Config.Principals {
		keys[p.ID] = p.PublicKey
		kinds[p.ID] = p.Kind
	}
	rb := a.RunbookSpec()
	return core.ApprovalSatisfied(rb.PolicyFor(req.Intent), rb.QuorumFor(req.Intent), req.Payload, req.Intent.Requester, a.Approvers, keys, kinds, req.Approvals, "")
}
func (b *Broker) runCount(st *core.State, i core.Intent, now int64) int {
	count := 0
	for _, r := range st.Requests {
		if r.Intent.Target == i.Target && r.Intent.Action == i.Action && r.Intent.Requester == i.Requester {
			for _, e := range r.Executions {
				if e.StartedAt > now-60 {
					count++
				}
			}
		}
	}
	return count
}
func (b *Broker) createRequest(w http.ResponseWriter, r *http.Request, p core.Principal) (any, error) {
	if !core.HasRole(p, "requester") {
		return nil, bad(403, "requester role required")
	}
	var in struct {
		Target    string                    `json:"target"`
		Action    string                    `json:"action"`
		Reason    string                    `json:"reason"`
		TTL       int                       `json:"ttl_seconds"`
		MaxUses   int                       `json:"max_uses"`
		Steps     []string                  `json:"steps"`
		Operation *core.DiagnosticOperation `json:"operation"`
	}
	if e := decode(w, r, &in); e != nil {
		return nil, e
	}
	t, a, ok := b.Config.Action(in.Target, in.Action)
	if !ok || !slices.Contains(t.Requesters, p.ID) {
		return nil, bad(403, "target/action not allowed")
	}
	policy := a.Review.Effective()
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Reason == "" && policy.Mode == "auto" {
		in.Reason = "Routine diagnostic under administrator preauthorization"
	}
	if in.Reason == "" && in.Operation != nil {
		in.Reason = "Terminal diagnostic within administrator-installed scope"
	}
	if len(in.Reason) < 8 || len(in.Reason) > 1000 || strings.ContainsAny(in.Reason, "\x00\x1b") {
		return nil, bad(400, "reason must contain 8–1000 bytes, without control escapes")
	}
	if in.TTL == 0 {
		in.TTL = 300
	}
	if in.TTL < 30 || in.TTL > core.MaxTTL {
		return nil, bad(400, "TTL must be 30–900 seconds")
	}
	if in.MaxUses == 0 {
		in.MaxUses = 1
	}
	if in.MaxUses < 1 || in.MaxUses > policy.MaxUses {
		return nil, bad(403, "requested execution count exceeds administrator policy")
	}
	now := b.Now()
	i := core.Intent{Version: 2, WorkspaceID: b.Config.WorkspaceID, ProjectID: t.ProjectID, ID: core.ID(), Target: t.ID, Action: a.ID, CommandHash: a.CommandHash, Requester: p.ID, Reason: in.Reason, CreatedAt: now.Unix(), ExpiresAt: now.Unix() + int64(in.TTL), MaxUses: in.MaxUses}
	if i.WorkspaceID != "" || i.ProjectID != "" {
		i.Version = 3
	}
	rb := a.RunbookSpec()
	if rb.Kind() != "task" {
		i.Version = 4
		i.Kind = rb.Kind()
		if a.Diagnostics != nil {
			i.Scope = a.Diagnostics
			if in.Operation != nil {
				i.Kind = "command"
				i.Operation = in.Operation
			}
		}
		if a.Plan != nil {
			var err error
			i.Plan, err = a.Plan.Select(in.Steps, a.Timeout)
			if err != nil {
				return nil, bad(400, err.Error())
			}
			if in.TTL < a.Timeout+5 {
				return nil, bad(400, "TTL must cover the plan execution time plus 5 seconds")
			}
		}
		if err := rb.ValidateIntent(i); err != nil {
			return nil, bad(400, err.Error())
		}
	} else if in.Operation != nil || len(in.Steps) > 0 {
		return nil, bad(400, "unexpected workflow input")
	}
	if a.Plan == nil && len(in.Steps) > 0 {
		return nil, bad(400, "steps require a change plan")
	}
	if a.Diagnostics == nil && in.Operation != nil {
		return nil, bad(400, "operation requires diagnostic scope")
	}
	policy = rb.PolicyFor(i)
	req := core.Request{Operation: i.Operation, Intent: i, Payload: core.Encode(i), Fingerprint: core.Hash(core.JSON(i)), Status: "pending", Approvals: []core.Approval{}, Quorum: rb.QuorumFor(i), Review: policy, Queued: i.Kind != "session", Assessments: []core.Assessment{}, Executions: []core.Execution{}}
	if b.Config.DevicePolicy != nil && b.Config.DevicePolicy.Required {
		req.DeviceID = r.Header.Get("X-Safe-Devops-Device")
	}
	if policy.Mode == "auto" {
		req.Status = "approved"
	}
	e := b.transaction(r.Context(), func(st *core.State) error {
		if locked(st, p.ID, now.Unix()) {
			return bad(423, "account temporarily locked by risk review; contact an independent human reviewer")
		}
		active, recent := 0, 0
		for _, v := range st.Requests {
			if v.Intent.Requester == p.ID {
				if slices.Contains([]string{"pending", "approved", "running", "ready"}, v.Status) {
					active++
				}
				if v.Intent.Target == t.ID && v.Intent.Action == a.ID && v.Intent.CreatedAt > now.Unix()-60 {
					recent++
				}
			}
		}
		if active >= 20 || recent >= policy.RateLimit {
			return bad(429, "active request or per-task rate limit exceeded")
		}
		req.Context = core.CollectRiskContext(st, i, now.Unix())
		st.Requests = append(st.Requests, req)
		store.Append(st, now, p.ID, "request.created", i.ID, req.Fingerprint)
		if req.DeviceID != "" {
			store.Append(st, now, p.ID, "request.device", i.ID, req.DeviceID)
		}
		if policy.Mode == "auto" {
			store.Append(st, now, "policy", "request.auto_authorized", i.ID, "low-risk local policy; one execution")
		}
		return nil
	})
	return req, e
}
func (b *Broker) reviewQueue(r *http.Request, p core.Principal) (any, error) {
	if core.Kind(p.Kind) != "ai" || !core.HasRole(p, "approver") {
		return nil, bad(403, "AI reviewer identity required")
	}
	out := []core.Request{}
	e := b.transaction(r.Context(), func(st *core.State) error {
		for _, req := range st.Requests {
			if req.Status != "pending" || req.NeedsHuman || req.Review.Mode != "ai" || !visible(b.Config, p, req) {
				continue
			}
			seen := false
			for _, v := range req.Assessments {
				if v.Reviewer == p.ID {
					seen = true
				}
			}
			if !seen {
				out = append(out, req)
			}
			if len(out) >= 20 {
				break
			}
		}
		return nil
	})
	return map[string]any{"requests": out}, e
}
func (b *Broker) requestAction(w http.ResponseWriter, r *http.Request, p core.Principal) (any, error) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/requests/"), "/")
	if len(parts) > 2 || parts[0] == "" {
		return nil, bad(404, "not found")
	}
	var sig struct {
		Signature string `json:"signature"`
	}
	var operation core.DiagnosticOperation
	var decision core.ReviewSubmission
	var rejection struct {
		Reason string `json:"reason"`
	}
	if len(parts) == 2 && r.Method == "POST" {
		switch parts[1] {
		case "command":
			if e := decode(w, r, &operation); e != nil {
				return nil, e
			}
		case "approve":
			if e := decode(w, r, &sig); e != nil {
				return nil, e
			}
		case "review":
			if e := decode(w, r, &decision); e != nil {
				return nil, e
			}
		case "reject":
			if e := decode(w, r, &rejection); e != nil {
				return nil, e
			}
		}
	}
	var out core.Request
	e := b.transaction(r.Context(), func(st *core.State) error {
		req, e := find(st, parts[0])
		if e != nil {
			return e
		}
		if !visible(b.Config, p, *req) {
			return bad(403, "request access denied")
		}
		if len(parts) == 1 && r.Method == "GET" {
			out = *req
			return nil
		}
		if len(parts) != 2 || r.Method != "POST" {
			return bad(404, "not found")
		}
		if locked(st, p.ID, b.Now().Unix()) && parts[1] != "revoke" {
			return bad(423, "account locked by risk review")
		}
		_, a, _ := b.Config.Action(req.Intent.Target, req.Intent.Action)
		policy := a.RunbookSpec().PolicyFor(req.Intent)
		human := core.Kind(p.Kind) == "human" && core.HasRole(p, "approver") && p.ID != req.Intent.Requester && slices.Contains(a.Approvers, p.ID)
		switch parts[1] {
		case "approve":
			if !human {
				return bad(403, "independent human approver required")
			}
			if req.Status != "pending" {
				return bad(409, "request is not pending")
			}
			if e := b.addApproval(st, req, a, p, sig.Signature); e != nil {
				return e
			}
		case "review":
			if core.Kind(p.Kind) != "ai" || !core.HasRole(p, "approver") || !slices.Contains(a.Approvers, p.ID) || p.ID == req.Intent.Requester || policy.Mode != "ai" || policy.Risk == "high" {
				return bad(403, "AI approval is not allowed by this task policy")
			}
			if req.Status != "pending" || req.NeedsHuman {
				return bad(409, "request no longer accepts AI review")
			}
			for _, v := range req.Assessments {
				if v.Reviewer == p.ID {
					return bad(409, "review already recorded")
				}
			}
			if !slices.Contains([]string{"approve", "escalate", "reject", "lock"}, decision.Decision) || len(strings.TrimSpace(decision.Reason)) < 3 || len(decision.Reason) > 2000 || len(decision.Model) < 1 || len(decision.Model) > 200 {
				return bad(400, "invalid review decision")
			}
			if req.Context != nil && decision.ContextDigest != req.Context.Digest() {
				return bad(409, "risk context digest mismatch; fetch the review queue again")
			}
			if decision.Decision == "lock" {
				if !core.Verify(p.PublicKey, core.RiskDecisionDomain, core.RiskDecisionPayload(req.Intent.ID, decision), decision.Signature) {
					return bad(403, "signed risk-lock decision required")
				}
				b.lockAccount(st, req.Intent, p.ID, decision)
			}
			if decision.Decision == "approve" {
				if e := b.addApproval(st, req, a, p, decision.Signature); e != nil {
					return e
				}
			} else {
				req.NeedsHuman = true
			}
			req.Assessments = append(req.Assessments, core.Assessment{ContextDigest: decision.ContextDigest, Reviewer: p.ID, Decision: decision.Decision, Reason: decision.Reason, Model: decision.Model, At: b.Now().Unix()})
			store.Append(st, b.Now(), p.ID, "review."+decision.Decision, req.Intent.ID, decision.Reason)
		case "reject":
			if !human {
				return bad(403, "independent human approver required")
			}
			if req.Status != "pending" {
				return bad(409, "request is not pending")
			}
			if len(strings.TrimSpace(rejection.Reason)) < 3 || len(rejection.Reason) > 1000 {
				return bad(400, "rejection reason required")
			}
			req.Status = "rejected"
			req.Queued = false
			req.Assessments = append(req.Assessments, core.Assessment{Reviewer: p.ID, Decision: "reject", Reason: rejection.Reason, Model: "human", At: b.Now().Unix()})
			store.Append(st, b.Now(), p.ID, "request.rejected", req.Intent.ID, rejection.Reason)
		case "handoff":
			if req.Intent.Requester != p.ID && !human {
				return bad(403, "handoff not allowed")
			}
			if req.Status != "pending" || policy.Mode != "ai" {
				return bad(409, "AI request is not pending")
			}
			req.NeedsHuman = true
			store.Append(st, b.Now(), p.ID, "review.handoff", req.Intent.ID, "manual review requested")
		case "command":
			if req.Intent.Requester != p.ID {
				return bad(403, "only the requester may submit session commands")
			}
			if req.Intent.Kind != "session" || a.Diagnostics == nil {
				return bad(409, "not a diagnostic session")
			}
			if req.Status != "ready" || req.Used >= core.Uses(req.Intent) {
				return bad(409, "session unavailable or exhausted")
			}
			if e := a.Diagnostics.ValidateOperation(operation); e != nil {
				return bad(400, e.Error())
			}
			if !b.satisfied(*req, a) {
				return bad(403, "session approval unavailable")
			}
			if b.runCount(st, req.Intent, b.Now().Unix()) >= policy.RateLimit {
				return bad(429, "session rate limit exceeded")
			}
			req.Operation = &operation
			req.Status = "approved"
			req.Queued = true
			store.Append(st, b.Now(), p.ID, "session.command_queued", req.Intent.ID, core.Hash(core.JSON(operation)))
		case "execute":
			if req.Intent.Kind == "session" {
				return bad(409, "submit an exact session command instead")
			}
			if req.Intent.Requester != p.ID {
				return bad(403, "only the original requester may reuse this grant")
			}
			if req.Status != "ready" || req.Used >= core.Uses(req.Intent) {
				return bad(409, "grant is unavailable or exhausted")
			}
			if !b.satisfied(*req, a) {
				return bad(403, "approval policy no longer satisfied")
			}
			if b.runCount(st, req.Intent, b.Now().Unix()) >= policy.RateLimit {
				return bad(429, "per-task execution rate limit exceeded")
			}
			req.Status = "approved"
			req.Queued = true
			store.Append(st, b.Now(), p.ID, "execution.queued", req.Intent.ID, "reuse within signed execution budget")
		case "revoke":
			if p.ID != req.Intent.Requester && !human {
				return bad(403, "revocation not permitted")
			}
			if !slices.Contains([]string{"pending", "approved", "ready", "running"}, req.Status) {
				return bad(409, "request is already terminal")
			}
			req.Status = "revoked"
			req.Queued = false
			store.Append(st, b.Now(), p.ID, "request.revoked", req.Intent.ID, "best-effort cancellation; completed effects cannot be undone")
		default:
			return bad(404, "not found")
		}
		out = *req
		return nil
	})
	return out, e
}
func (b *Broker) addApproval(st *core.State, req *core.Request, a core.Action, p core.Principal, signature string) error {
	for _, v := range req.Approvals {
		if v.Subject == p.ID {
			return bad(409, "already approved")
		}
	}
	if !core.Verify(p.PublicKey, core.ApprovalDomain, req.Payload, signature) {
		return bad(403, "invalid signature for this exact intent")
	}
	req.Approvals = append(req.Approvals, core.Approval{Subject: p.ID, Signature: signature, At: b.Now().Unix(), Kind: core.Kind(p.Kind)})
	if b.satisfied(*req, a) {
		req.Status = "approved"
		if req.Intent.Kind == "session" {
			req.Status = "ready"
			req.Queued = false
		}
		req.NeedsHuman = false
	}
	store.Append(st, b.Now(), p.ID, "request.approved", req.Intent.ID, req.Fingerprint)
	return nil
}
