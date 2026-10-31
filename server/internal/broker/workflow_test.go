package broker

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"safeops/internal/agent"
	"safeops/internal/core"
	"safeops/internal/reviewer"
)

func workflowFixture(t *testing.T) *fixture {
	return setup(t, func(c *core.Config, ac *core.AgentConfig, keys map[string]ed25519.PrivateKey) {
		pub, key, _ := ed25519.GenerateKey(rand.Reader)
		keys["machine"] = key
		public := base64.StdEncoding.EncodeToString(pub)
		c.Principals = append(c.Principals, core.Principal{ID: "machine", Kind: "ai", Roles: []string{"approver"}, PublicKey: public, TokenHash: core.Hash([]byte("machine"))})
		c.WorkspaceID = "workspace"
		c.Projects = []core.Project{{ID: "project", Name: "Project", Members: []string{"alice", "bob", "carol", "audit", "machine"}}}
		ac.WorkspaceID = "workspace"
		ac.ProjectID = "project"
		ac.ApproverKeys["machine"] = public
		ac.ApproverKinds = map[string]string{"machine": "ai"}
		diagnostics := core.Runbook{Timeout: 15, Diagnostics: &core.DiagnosticPolicy{Roots: []core.DiagnosticRoot{{ID: "logs", Path: t.TempDir()}}, Tools: []string{"ls", "cat", "status"}, AutoTools: []string{"ls", "status"}}, Quorum: 1, Approvers: []string{"bob", "carol", "machine"}, Requesters: []string{"alice"}, Review: core.ReviewPolicy{Mode: "ai", Risk: "medium", MaxUses: 100, RateLimit: 60}}
		plan := core.Runbook{Timeout: 20, Plan: &core.PlanPolicy{Privileged: true, MaxSteps: 2, Steps: []core.ChangeStep{{ID: "one", Name: "one", Argv: []string{"/bin/echo", "first"}, Timeout: 5}, {ID: "two", Name: "two", Argv: []string{"/bin/echo", "second"}, Timeout: 5}}}, Quorum: 2, Approvers: []string{"bob", "carol"}, Requesters: []string{"alice"}, Review: core.ReviewPolicy{Mode: "manual", Risk: "high", MaxUses: 1, RateLimit: 6}}
		ac.Runbooks = map[string]core.Runbook{"diagnostics": diagnostics, "changes": plan}
		ac.ChangeSocket = filepath.Join(t.TempDir(), "runner.sock")
		for n := range c.Targets {
			c.Targets[n].ProjectID = "project"
			c.Targets[n].Actions = nil
			for id, rb := range ac.Runbooks {
				c.Targets[n].Actions = append(c.Targets[n].Actions, core.Action{ID: id, CommandHash: rb.Hash(), Timeout: rb.Timeout, Diagnostics: rb.Diagnostics, Plan: rb.Plan, Quorum: rb.Quorum, Approvers: rb.Approvers, Review: rb.Review})
			}
		}
	})
}
func (f *fixture) command(line string) core.Request {
	var r core.Request
	f.call("alice", "POST", "/api/terminal/command", map[string]any{"target": "t1", "profile": "diagnostics", "command": line}, 200, &r)
	return r
}
func TestTerminalCascadeAndExactCommand(t *testing.T) {
	f := workflowFixture(t)
	fast := f.command("ls")
	if fast.Status != "approved" || fast.Review.Mode != "auto" {
		t.Fatal("simple command not automatic")
	}
	env := f.poll("agent")
	if env == nil {
		t.Fatal("missing command lease")
	}
	if _, _, e := agent.Verify(f.config, *env, time.Now()); e != nil {
		t.Fatal(e)
	}
	// Even a holder of the Broker key cannot turn ls into file disclosure.
	altered := *env
	op := *env.Lease.Operation
	op.Tool = "cat"
	op.Path = "secrets"
	altered.Lease.Operation = &op
	altered.Signature = core.Sign(f.keys["broker"], core.LeaseDomain, core.Encode(altered.Lease))
	if _, _, e := agent.Verify(f.config, altered, time.Now()); e == nil {
		t.Fatal("altered automatic operation executed")
	}
	f.result(fast, env, 0)
	reviewed := f.command("cat logs:app.log")
	if reviewed.Status != "pending" || reviewed.Review.Mode != "ai" || len(reviewed.Context.Records) != 1 {
		t.Fatal("missing AI context")
	}
	f.call("machine", "POST", "/api/requests/"+reviewed.Intent.ID+"/review", core.ReviewSubmission{Decision: "approve", Reason: "reviewed", Model: "test", Signature: core.Sign(f.keys["machine"], core.ApprovalDomain, reviewed.Payload), ContextDigest: "wrong"}, 409, nil)
	f.call("machine", "POST", "/api/requests/"+reviewed.Intent.ID+"/review", core.ReviewSubmission{Decision: "escalate", Reason: "Recent activity is uncertain", Model: "test", ContextDigest: reviewed.Context.Digest()}, 200, &reviewed)
	if !reviewed.NeedsHuman || f.poll("agent") != nil {
		t.Fatal("handoff executed")
	}
	f.approve(reviewed, "bob", 200)
	env = f.poll("agent")
	if env == nil {
		t.Fatal("human fallback failed")
	}
	if _, _, e := agent.Verify(f.config, *env, time.Now()); e != nil {
		t.Fatal(e)
	}
}
func TestSessionCommandsRequireReadyGrantAndStayScoped(t *testing.T) {
	f := workflowFixture(t)
	var r core.Request
	f.call("alice", "POST", "/api/requests", map[string]any{"target": "t1", "action": "diagnostics", "reason": "Investigate app failures", "max_uses": 3}, 200, &r)
	path := "/api/requests/" + r.Intent.ID
	op := core.DiagnosticOperation{Tool: "cat", Resource: "logs", Path: "missing.log"}
	f.call("alice", "POST", path+"/command", op, 409, nil)
	f.approve(r, "bob", 200)
	if f.poll("agent") != nil {
		t.Fatal("session approval automatically executed")
	}
	f.call("bob", "POST", path+"/command", op, 403, nil)
	f.call("alice", "POST", path+"/execute", map[string]any{}, 409, nil)
	f.call("alice", "POST", path+"/command", core.DiagnosticOperation{Tool: "cat", Resource: "logs", Path: "../escape"}, 400, nil)
	f.call("alice", "POST", path+"/command", op, 200, nil)
	env := f.poll("agent")
	if env == nil {
		t.Fatal("no session operation")
	}
	if _, _, e := agent.Verify(f.config, *env, time.Now()); e != nil {
		t.Fatal(e)
	}
	f.result(r, env, 1)
	f.call("alice", "GET", path, nil, 200, &r)
	if r.Status != "ready" || r.Executions[0].Status != "failed" {
		t.Fatal("failed read terminated session")
	}
	f.call("alice", "POST", path+"/revoke", map[string]any{}, 200, nil)
	f.call("alice", "POST", path+"/command", op, 409, nil)
}
func TestPrivilegedRunnerIndependentlyVerifiesPlanAndReplay(t *testing.T) {
	f := workflowFixture(t)
	var r core.Request
	f.call("alice", "POST", "/api/requests", map[string]any{"target": "t1", "action": "changes", "reason": "Approved maintenance plan", "steps": []string{"one", "two"}}, 200, &r)
	f.approve(r, "bob", 200)
	if f.poll("agent") != nil {
		t.Fatal("one human authorized privileged plan")
	}
	f.approve(r, "carol", 200)
	env := f.poll("agent")
	if env == nil {
		t.Fatal("missing plan lease")
	}
	changed := *env
	i, _ := core.DecodeIntent(env.Lease.Payload)
	i.Plan = append([]core.ChangeStep{}, i.Plan...)
	i.Plan[0], i.Plan[1] = i.Plan[1], i.Plan[0]
	changed.Lease.Payload = core.Encode(i)
	changed.Signature = core.Sign(f.keys["broker"], core.LeaseDomain, core.Encode(changed.Lease))
	if _, _, e := agent.Verify(f.config, changed, time.Now()); e == nil {
		t.Fatal("reordered signed plan accepted")
	}
	c := f.config
	c.BrokerURL = f.server.URL
	c.DemoToken = "agent"
	ledger, e := agent.OpenLedger(filepath.Join(t.TempDir(), "runner-ledger"))
	if e != nil {
		t.Fatal(e)
	}
	defer ledger.Close()
	handler := agent.RunnerHandler(c, http.DefaultClient, ledger)
	for n, want := range []int{200, 403} {
		req := httptest.NewRequest("POST", "/execute", bytes.NewReader(core.JSON(env)))
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("attempt%d got%d: %s", n, res.Code, res.Body.String())
		}
		if n == 0 {
			var result struct {
				Output string `json:"output"`
				Exit   int    `json:"exit_code"`
			}
			if json.Unmarshal(res.Body.Bytes(), &result) != nil || result.Exit != 0 || result.Output == "" {
				t.Fatal("runner failed")
			}
		}
	}
}
func TestAIContextSnapshotAndTimeoutHandoff(t *testing.T) {
	f := workflowFixture(t)
	r := f.command("cat logs:app.log")
	var queue struct {
		Requests []core.Request `json:"requests"`
	}
	f.call("machine", "GET", "/api/reviewer/queue", nil, 200, &queue)
	fast := f.command("status")
	env := f.poll("agent")
	f.result(fast, env, 0)
	f.call("machine", "GET", "/api/reviewer/queue", nil, 200, &queue)
	if len(queue.Requests) != 1 || queue.Requests[0].Context.Digest() != r.Context.Digest() {
		t.Fatal("review snapshot changed while model ran")
	}
	f.broker.Now = func() time.Time { return time.Unix(r.Intent.CreatedAt+46, 0) }
	f.call("alice", "GET", "/api/requests/"+r.Intent.ID, nil, 200, &r)
	if !r.NeedsHuman || len(r.Assessments) != 1 {
		t.Fatal("offline reviewer not handed off")
	}
}
func TestModelReceivesRecentOutputAndContextDigest(t *testing.T) {
	f := workflowFixture(t)
	r := f.command("status")
	env := f.poll("agent")
	f.call("agent", "POST", "/api/agent/result", map[string]any{"request_id": r.Intent.ID, "lease_id": env.Lease.ID, "exit_code": 1, "output": "upstream failed; token=DO_NOT_SEND"}, 200, nil)
	next := f.command("cat logs:app.log")
	seen := false
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		var evidence struct {
			Risk *core.RiskContext `json:"recent_activity_untrusted"`
		}
		_ = json.Unmarshal([]byte(body.Messages[1].Content), &evidence)
		if evidence.Risk == nil || len(evidence.Risk.Records) != 1 || evidence.Risk.Records[0].Output != "upstream failed; [REDACTED]" {
			t.Error("missing or unredacted evidence")
		}
		seen = true
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": `{"decision":"escalate","reason":"Prior upstream failure needs investigation","confidence":0.9}`}}}})
	}))
	defer model.Close()
	t.Setenv("TEST_MODEL_KEY", "test")
	cfg := reviewer.Config{WorkspaceID: "workspace", Model: "mock", Endpoint: model.URL, APIKeyEnv: "TEST_MODEL_KEY", BrokerURL: f.server.URL, MinConfidence: .9, Rules: []reviewer.Rule{{ProjectID: "project", Target: "t1", Action: "diagnostics", CommandHash: next.Intent.CommandHash, Requesters: []string{"alice"}, Risk: "medium", MaxTTL: 900, MaxUses: 100}}}
	provider, e := reviewer.NewHTTPProvider(cfg, true)
	if e != nil {
		t.Fatal(e)
	}
	worker := reviewer.Worker{Config: cfg, Key: f.keys["machine"], Provider: provider, Now: time.Now}
	if e = worker.ProcessOnce(context.Background(), http.DefaultClient, "machine"); e != nil {
		t.Fatal(e)
	}
	if !seen {
		t.Fatal("model was not called")
	}
	f.call("alice", "GET", "/api/requests/"+next.Intent.ID, nil, 200, &next)
	if !next.NeedsHuman || next.Assessments[0].ContextDigest != next.Context.Digest() {
		t.Fatal("review trace missing")
	}
}
