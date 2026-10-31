package broker

import (
	"net/http"
	"slices"
	"strings"

	"safeops/internal/core"
	"safeops/internal/store"
)

func locked(st *core.State, id string, now int64) bool {
	for _, l := range st.AccountLocks {
		if l.Requester == id && l.Active(now) {
			return true
		}
	}
	return false
}
func (b *Broker) lockAccount(st *core.State, i core.Intent, reviewer string, d core.ReviewSubmission) {
	if locked(st, i.Requester, b.Now().Unix()) {
		return
	}
	seconds := b.Config.RiskLockSeconds
	if seconds == 0 {
		seconds = 900
	}
	l := core.AccountLock{ID: core.ID(), WorkspaceID: b.Config.WorkspaceID, Requester: i.Requester, ProjectID: i.ProjectID, Target: i.Target, Action: i.Action, SourceID: i.ID, Reviewer: reviewer, Reason: d.Reason, Model: d.Model, ContextDigest: d.ContextDigest, CreatedAt: b.Now().Unix(), ExpiresAt: b.Now().Unix() + int64(seconds)}
	st.AccountLocks = append(st.AccountLocks, l)
	for n := range st.Requests {
		req := &st.Requests[n]
		if req.Intent.Requester == i.Requester && slices.Contains([]string{"pending", "approved", "ready", "running"}, req.Status) {
			req.Status = "revoked"
			req.Queued = false
			store.Append(st, b.Now(), reviewer, "request.risk_revoked", req.Intent.ID, l.ID)
		}
	}
	// Old pending scans cannot relock this account after a human releases it.
	for n := range st.RiskScans {
		scan := &st.RiskScans[n]
		if scan.Requester == i.Requester && scan.Assessment == nil {
			scan.Assessment = &core.Assessment{Reviewer: "system", Decision: "superseded", Reason: "Closed by account lock " + l.ID, Model: "system", At: b.Now().Unix(), ContextDigest: scan.Context.Digest()}
		}
	}
	store.Append(st, b.Now(), reviewer, "account.locked", i.ID, string(core.JSON(l)))
}
func (b *Broker) lockReviewer(p core.Principal, l core.AccountLock) bool {
	_, a, ok := b.Config.Action(l.Target, l.Action)
	return ok && core.Kind(p.Kind) == "human" && core.HasRole(p, "approver") && p.ID != l.Requester && slices.Contains(a.Approvers, p.ID)
}
func (b *Broker) accountLocks(w http.ResponseWriter, r *http.Request, p core.Principal) (any, error) {
	if core.Kind(p.Kind) == "ai" {
		return nil, bad(403, "AI cannot unlock accounts")
	}
	var in struct {
		Signature string `json:"signature"`
	}
	if r.Method == "POST" {
		if e := decode(w, r, &in); e != nil {
			return nil, e
		}
	}
	out := []core.AccountLock{}
	e := b.transaction(r.Context(), func(st *core.State) error {
		if r.Method == "GET" && r.URL.Path == "/api/account-locks" {
			for _, l := range st.AccountLocks {
				if l.Requester == p.ID || core.HasRole(p, "auditor") || b.lockReviewer(p, l) {
					out = append(out, l)
				}
			}
			if len(out) > 200 {
				out = out[len(out)-200:]
			}
			return nil
		}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/account-locks/"), "/")
		if r.Method != "POST" || len(parts) != 2 || parts[1] != "unlock" {
			return bad(404, "not found")
		}
		if locked(st, p.ID, b.Now().Unix()) {
			return bad(423, "locked identities cannot unlock accounts")
		}
		for n := range st.AccountLocks {
			l := &st.AccountLocks[n]
			if l.ID != parts[0] {
				continue
			}
			if !b.lockReviewer(p, *l) {
				return bad(403, "independent authorized human required")
			}
			if !l.Active(b.Now().Unix()) {
				return bad(409, "lock already released or expired")
			}
			if !core.Verify(p.PublicKey, core.UnlockDomain, l.Payload(), in.Signature) {
				return bad(403, "independent signature required for this exact lock")
			}
			l.ReleasedAt = b.Now().Unix()
			l.ReleasedBy = p.ID
			store.Append(st, b.Now(), p.ID, "account.unlocked", l.ID, l.Requester)
			out = append(out, *l)
			return nil
		}
		return bad(404, "lock not found")
	})
	items := []map[string]any{}
	for _, l := range out {
		items = append(items, map[string]any{"lock": l, "payload": l.Payload(), "fingerprint": core.Hash(core.JSON(l)), "can_unlock": b.lockReviewer(p, l)})
	}
	return map[string]any{"locks": items}, e
}

// Result delivery creates one pending snapshot per subject/target. Additional
// results are coalesced by generating a fresh scan after the pending one finishes.
func (b *Broker) scheduleRiskScan(st *core.State, req core.Request) {
	if locked(st, req.Intent.Requester, b.Now().Unix()) || len(req.Executions) == 0 {
		return
	}
	cursor := req.Executions[len(req.Executions)-1].LeaseID
	for _, v := range st.RiskScans {
		if v.Requester == req.Intent.Requester && v.Target == req.Intent.Target {
			if v.Cursor == cursor || (v.Assessment == nil && v.ExpiresAt > b.Now().Unix()) {
				return
			}
		}
	}
	target, _, ok := b.Config.Action(req.Intent.Target, req.Intent.Action)
	if !ok {
		return
	}
	var action *core.Action
	for n := range target.Actions {
		if target.Actions[n].Review.Effective().Mode == "ai" {
			action = &target.Actions[n]
			break
		}
	}
	if action == nil {
		return
	}
	i := req.Intent
	i.ID = "" // Include the just-finished execution in risk evidence.
	st.RiskScans = append(st.RiskScans, core.RiskScan{ID: core.ID(), WorkspaceID: b.Config.WorkspaceID, ProjectID: i.ProjectID, Target: i.Target, Action: action.ID, CommandHash: action.CommandHash, Requester: i.Requester, Cursor: cursor, CreatedAt: b.Now().Unix(), ExpiresAt: b.Now().Unix() + 120, Context: core.CollectRiskContext(st, i, b.Now().Unix())})
}
func (b *Broker) riskQueue(r *http.Request, p core.Principal) (any, error) {
	if core.Kind(p.Kind) != "ai" || !core.HasRole(p, "approver") {
		return nil, bad(403, "AI reviewer required")
	}
	out := []core.RiskScan{}
	e := b.transaction(r.Context(), func(st *core.State) error {
		// Refresh work from the most recent execution per subject/target, including
		// executions that arrived while the previous snapshot was being assessed.
		seen := map[string]bool{}
		for n := len(st.Audit) - 1; n >= 0; n-- {
			event := st.Audit[n]
			if event.Event != "execution.result" || event.At < b.Now().Unix()-86400 {
				continue
			}
			req, e := find(st, event.RequestID)
			if e != nil {
				continue
			}
			key := req.Intent.Requester + "/" + req.Intent.Target
			if seen[key] {
				continue
			}
			seen[key] = true
			b.scheduleRiskScan(st, *req)
		}

		for _, scan := range st.RiskScans {
			_, a, ok := b.Config.Action(scan.Target, scan.Action)
			if ok && scan.Assessment == nil && scan.ExpiresAt > b.Now().Unix() && !locked(st, scan.Requester, b.Now().Unix()) && slices.Contains(a.Approvers, p.ID) {
				out = append(out, scan)
				if len(out) >= 20 {
					break
				}
			}
		}
		return nil
	})
	return map[string]any{"scans": out}, e
}
func (b *Broker) riskDecision(w http.ResponseWriter, r *http.Request, p core.Principal) (any, error) {
	if core.Kind(p.Kind) != "ai" || !core.HasRole(p, "approver") {
		return nil, bad(403, "AI reviewer required")
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/reviewer/risk-scans/")
	var d core.ReviewSubmission
	if e := decode(w, r, &d); e != nil {
		return nil, e
	}
	if !slices.Contains([]string{"approve", "escalate", "reject", "lock"}, d.Decision) || len(strings.TrimSpace(d.Reason)) < 3 || len(d.Reason) > 2000 || len(d.Model) < 1 || len(d.Model) > 200 {
		return nil, bad(400, "invalid risk assessment")
	}
	e := b.transaction(r.Context(), func(st *core.State) error {
		for n := range st.RiskScans {
			scan := &st.RiskScans[n]
			if scan.ID != id {
				continue
			}
			_, a, ok := b.Config.Action(scan.Target, scan.Action)
			if !ok || !slices.Contains(a.Approvers, p.ID) || scan.Requester == p.ID {
				return bad(403, "risk scan access denied")
			}
			if scan.Assessment != nil || scan.ExpiresAt <= b.Now().Unix() {
				return bad(409, "scan no longer pending")
			}
			if d.ContextDigest != scan.Context.Digest() || !core.Verify(p.PublicKey, core.RiskDecisionDomain, core.RiskDecisionPayload(scan.ID, d), d.Signature) {
				return bad(403, "signed context-bound risk decision required")
			}
			scan.Assessment = &core.Assessment{Reviewer: p.ID, Decision: d.Decision, Reason: d.Reason, Model: d.Model, ContextDigest: d.ContextDigest, At: b.Now().Unix()}
			store.Append(st, b.Now(), p.ID, "risk."+d.Decision, scan.ID, string(core.JSON(scan.Assessment)))
			if d.Decision == "lock" {
				b.lockAccount(st, core.Intent{ID: scan.ID, ProjectID: scan.ProjectID, Target: scan.Target, Action: scan.Action, Requester: scan.Requester}, p.ID, d)
			}
			return nil
		}
		return bad(404, "scan not found")
	})
	return map[string]bool{"ok": e == nil}, e
}

func (b *Broker) riskHistory(r *http.Request, p core.Principal) (any, error) {
	out := []core.RiskScan{}
	e := b.transaction(r.Context(), func(st *core.State) error {
		for n := len(st.RiskScans) - 1; n >= 0 && len(out) < 50; n-- {
			scan := st.RiskScans[n]
			l := core.AccountLock{Requester: scan.Requester, Target: scan.Target, Action: scan.Action}
			if p.ID == scan.Requester || core.HasRole(p, "auditor") || b.lockReviewer(p, l) {
				out = append(out, scan)
			}
		}
		return nil
	})
	return map[string]any{"scans": out}, e
}
