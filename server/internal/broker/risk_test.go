package broker

import (
	"context"
	"net/http"
	"safeops/internal/core"
	"safeops/internal/reviewer"
	"testing"
	"time"
)

func (f *fixture) riskLock(id, digest string) core.ReviewSubmission {
	d := core.ReviewSubmission{Decision: "lock", Reason: "Test evidence of compromised identity", Model: "mock-risk", ContextDigest: digest}
	d.Signature = core.Sign(f.keys["machine"], core.RiskDecisionDomain, core.RiskDecisionPayload(id, d))
	return d
}
func TestAccountLockCancelsExistingGrantsAndRequiresSignedHumanUnlock(t *testing.T) {
	f := workflowFixture(t)
	running := f.command("status")
	lease := f.poll("agent")
	pending := f.command("cat logs:app.log")
	d := f.riskLock(pending.Intent.ID, pending.Context.Digest())
	unsigned := d
	unsigned.Signature = ""
	f.call("machine", "POST", "/api/requests/"+pending.Intent.ID+"/review", unsigned, 403, nil)
	f.call("machine", "POST", "/api/requests/"+pending.Intent.ID+"/review", d, 200, nil)
	var state struct {
		Active bool `json:"active"`
	}
	f.call("agent", "POST", "/api/agent/check", map[string]any{"request_id": running.Intent.ID, "lease_id": lease.Lease.ID}, 200, &state)
	if state.Active {
		t.Fatal("locked account retained running lease")
	}
	f.call("alice", "POST", "/api/terminal/command", map[string]any{"target": "t1", "profile": "diagnostics", "command": "status"}, 423, nil)
	var data struct {
		Locks []struct {
			Lock    core.AccountLock `json:"lock"`
			Payload string           `json:"payload"`
		} `json:"locks"`
	}
	f.call("bob", "GET", "/api/account-locks", nil, 200, &data)
	if len(data.Locks) != 1 {
		t.Fatal("missing account lock")
	}
	l := data.Locks[0]
	path := "/api/account-locks/" + l.Lock.ID + "/unlock"
	f.call("alice", "POST", path, map[string]string{"signature": core.Sign(f.keys["alice"], core.UnlockDomain, l.Payload)}, 423, nil)
	f.call("machine", "POST", path, map[string]string{"signature": core.Sign(f.keys["machine"], core.UnlockDomain, l.Payload)}, 403, nil)
	f.call("bob", "POST", path, map[string]string{"signature": ""}, 403, nil)
	f.call("bob", "POST", path, map[string]string{"signature": core.Sign(f.keys["bob"], core.ApprovalDomain, l.Payload)}, 403, nil)
	f.call("bob", "POST", path, map[string]string{"signature": core.Sign(f.keys["bob"], core.UnlockDomain, l.Payload)}, 200, nil)
	var old core.Request
	f.call("alice", "GET", "/api/requests/"+running.Intent.ID, nil, 200, &old)
	if old.Status != "revoked" {
		t.Fatal("unlock restored old authority")
	}
	f.command("status")
	f.call("bob", "POST", path, map[string]string{"signature": core.Sign(f.keys["bob"], core.UnlockDomain, l.Payload)}, 409, nil)
}
func TestRetrospectiveScanLocksAfterAutoCommands(t *testing.T) {
	f := workflowFixture(t)
	for n := 0; n < 5; n++ {
		r := f.command("status")
		lease := f.poll("agent")
		f.result(r, lease, 1)
	}
	cfg := reviewer.Config{WorkspaceID: "workspace", Simulation: true, MinConfidence: .9, BrokerURL: f.server.URL, Rules: []reviewer.Rule{{ProjectID: "project", Target: "t1", Action: "diagnostics", CommandHash: f.config.Runbooks["diagnostics"].Hash(), Requesters: []string{"alice"}, Risk: "medium", MaxTTL: 900, MaxUses: 100}}}
	worker := reviewer.Worker{Config: cfg, Key: f.keys["machine"], Now: time.Now}
	// First scan captured the first result. The next pass coalesces the later four.
	for n := 0; n < 2; n++ {
		if e := worker.ProcessRiskOnce(context.Background(), http.DefaultClient, "machine"); e != nil {
			t.Fatal(e)
		}
	}
	f.call("alice", "POST", "/api/terminal/command", map[string]any{"target": "t1", "profile": "diagnostics", "command": "ls"}, 423, nil)
	var scans struct {
		Scans []core.RiskScan `json:"scans"`
	}
	f.call("alice", "GET", "/api/risk-scans", nil, 200, &scans)
	if len(scans.Scans) != 2 || scans.Scans[0].Assessment.Decision != "lock" {
		t.Fatal("missing risk assessment trace")
	}
}
func TestAccountLockExpiresWithoutRestoringGrants(t *testing.T) {
	f := workflowFixture(t)
	r := f.command("cat logs:app.log")
	f.call("machine", "POST", "/api/requests/"+r.Intent.ID+"/review", f.riskLock(r.Intent.ID, r.Context.Digest()), 200, nil)
	f.broker.Now = func() time.Time { return time.Unix(r.Intent.CreatedAt+901, 0) }
	f.command("status")
	f.call("alice", "GET", "/api/requests/"+r.Intent.ID, nil, 200, &r)
	if r.Status != "revoked" {
		t.Fatal("expiry restored old request")
	}
}
