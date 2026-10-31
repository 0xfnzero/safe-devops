package broker

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"safeops/internal/agent"
	"safeops/internal/core"
	"safeops/internal/reviewer"
	"testing"
	"time"
)

func modeFixture(t *testing.T, mode string) *fixture {
	return setup(t, func(c *core.Config, ac *core.AgentConfig, keys map[string]ed25519.PrivateKey) {
		rb := ac.Runbooks["status"]
		rb.Review = core.ReviewPolicy{Mode: mode, Risk: "medium", MaxUses: 3, RateLimit: 6}
		rb.Quorum = 1
		rb.Approvers = []string{"bob", "carol"}
		if mode == "auto" {
			rb.Quorum = 0
			rb.Approvers = nil
			rb.Review.Risk = "low"
			rb.Review.MaxUses = 1
		}
		if mode == "ai" {
			pub, key, _ := ed25519.GenerateKey(rand.Reader)
			keys["machine"] = key
			public := base64.StdEncoding.EncodeToString(pub)
			c.Principals = append(c.Principals, core.Principal{ID: "machine", Kind: "ai", Roles: []string{"approver"}, TokenHash: core.Hash([]byte("machine")), PublicKey: public})
			rb.Approvers = append(rb.Approvers, "machine")
			ac.ApproverKeys["machine"] = public
			ac.ApproverKinds = map[string]string{"machine": "ai"}
		}
		ac.Runbooks["status"] = rb
		for n := range c.Targets {
			a := &c.Targets[n].Actions[0]
			a.Review = rb.Review
			a.Quorum = rb.Quorum
			a.Approvers = rb.Approvers
		}
	})
}
func (f *fixture) grant(uses int) core.Request {
	var r core.Request
	f.call("alice", "POST", "/api/requests", map[string]any{"target": "t1", "action": "status", "reason": "INC42 bounded diagnosis", "max_uses": uses, "ttl_seconds": 300}, 200, &r)
	return r
}
func (f *fixture) result(r core.Request, l *core.Envelope, code int) {
	f.call("agent", "POST", "/api/agent/result", map[string]any{"request_id": r.Intent.ID, "lease_id": l.Lease.ID, "exit_code": code, "output": "test output"}, 200, nil)
}
func TestAutomaticModeIsPreauthorizedAndRateLimited(t *testing.T) {
	f := modeFixture(t, "auto")
	r := f.grant(1)
	if r.Status != "approved" || len(r.Approvals) != 0 {
		t.Fatal("automatic authorization missing")
	}
	env := f.poll("agent")
	if env == nil {
		t.Fatal("automatic request not dispatched")
	}
	if _, _, e := agent.Verify(f.config, *env, time.Now()); e != nil {
		t.Fatal(e)
	}
	f.result(r, env, 0)
	for n := 0; n < 5; n++ {
		r = f.grant(1)
		env = f.poll("agent")
		if env == nil {
			t.Fatal("expected dispatch")
		}
		f.result(r, env, 0)
	}
	f.call("alice", "POST", "/api/requests", map[string]any{"target": "t1", "action": "status"}, 429, nil)
	f.call("alice", "POST", "/api/requests", map[string]any{"target": "t1", "action": "status", "review": map[string]string{"mode": "auto"}}, 400, nil)
}
func TestBoundedGrantReuseAndTerminalStates(t *testing.T) {
	for _, terminal := range []string{"exhausted", "failed", "revoked", "expired"} {
		t.Run(terminal, func(t *testing.T) {
			f := modeFixture(t, "manual")
			r := f.grant(2)
			f.approve(r, "bob", 200)
			env := f.poll("agent")
			if env == nil {
				t.Fatal("missing lease")
			}
			code := 0
			if terminal == "failed" {
				code = 1
			}
			f.result(r, env, code)
			path := "/api/requests/" + r.Intent.ID
			if f.poll("agent") != nil {
				t.Fatal("automatically repeated execution")
			}
			f.call("bob", "POST", path+"/execute", map[string]any{}, 403, nil)
			switch terminal {
			case "failed":
				f.call("alice", "POST", path+"/execute", map[string]any{}, 409, nil)
			case "revoked":
				f.call("alice", "POST", path+"/revoke", map[string]any{}, 200, nil)
				f.call("alice", "POST", path+"/execute", map[string]any{}, 409, nil)
			case "expired":
				f.broker.Now = func() time.Time { return time.Unix(r.Intent.ExpiresAt+1, 0) }
				f.call("alice", "POST", path+"/execute", map[string]any{}, 409, nil)
			case "exhausted":
				f.call("alice", "POST", path+"/execute", map[string]any{}, 200, nil)
				f.call("alice", "POST", path+"/execute", map[string]any{}, 409, nil)
				second := f.poll("agent")
				if second == nil || second.Lease.Use != 2 || second.Lease.Payload != env.Lease.Payload {
					t.Fatal("reuse changed signed intent")
				}
				if _, _, e := agent.Verify(f.config, *second, time.Now()); e != nil {
					t.Fatal(e)
				}
				f.result(r, second, 0)
				f.call("alice", "POST", path+"/execute", map[string]any{}, 409, nil)
				var done core.Request
				f.call("alice", "GET", path, nil, 200, &done)
				if len(done.Executions) != 2 || done.Status != "completed" {
					t.Fatal("execution history lost")
				}
			}
		})
	}
}
func TestAIEscalationHumanFallbackAndRejection(t *testing.T) {
	for _, decision := range []string{"approve", "escalate", "reject", "handoff", "human-reject"} {
		t.Run(decision, func(t *testing.T) {
			f := modeFixture(t, "ai")
			r := f.grant(1)
			path := "/api/requests/" + r.Intent.ID
			f.approve(r, "machine", 403)
			if decision == "handoff" {
				f.call("alice", "POST", path+"/handoff", map[string]any{}, 200, nil)
			} else if decision == "human-reject" {
				f.call("bob", "POST", path+"/reject", map[string]string{"reason": "Invalid maintenance request"}, 200, nil)
			} else {
				in := core.ReviewSubmission{ContextDigest: r.Context.Digest(), Decision: decision, Reason: "Test independent review", Model: "local-mock"}
				if decision == "approve" {
					in.Signature = core.Sign(f.keys["machine"], core.ApprovalDomain, r.Payload)
				}
				f.call("machine", "POST", path+"/review", in, 200, nil)
			}
			if decision == "approve" {
				env := f.poll("agent")
				if env == nil {
					t.Fatal("AI approved grant missing")
				}
				if _, _, e := agent.Verify(f.config, *env, time.Now()); e != nil {
					t.Fatal(e)
				}
				return
			}
			if f.poll("agent") != nil {
				t.Fatal("escalated request executed")
			}
			f.call("machine", "POST", path+"/review", core.ReviewSubmission{Decision: "approve", Reason: "Late model decision", Model: "mock", Signature: core.Sign(f.keys["machine"], core.ApprovalDomain, r.Payload)}, 409, nil)
			if decision == "human-reject" {
				f.approve(r, "bob", 409)
				return
			}
			f.approve(r, "bob", 200)
			if f.poll("agent") == nil {
				t.Fatal("human fallback failed")
			}
		})
	}
}
func TestModelReviewerToRealAgentIntegration(t *testing.T) {
	f := modeFixture(t, "ai")
	r := f.grant(2)
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": `{"decision":"approve","reason":"Diagnostic within independent policy","confidence":0.99}`}}}})
	}))
	defer model.Close()
	t.Setenv("SAFEOPS_TEST_AI_KEY", "local-fixture-key")
	config := reviewer.Config{Model: "mock-model", Endpoint: model.URL, APIKeyEnv: "SAFEOPS_TEST_AI_KEY", BrokerURL: f.server.URL, MinConfidence: 0.9, Rules: []reviewer.Rule{{Target: "t1", Action: "status", CommandHash: r.Intent.CommandHash, Requesters: []string{"alice"}, Risk: "medium", MaxTTL: 300, MaxUses: 3}}}
	provider, e := reviewer.NewHTTPProvider(config, true)
	if e != nil {
		t.Fatal(e)
	}
	worker := reviewer.Worker{Config: config, Key: f.keys["machine"], Provider: provider, Now: time.Now}
	client, e := agent.Client(f.server.URL, "", "", "", true)
	if e != nil {
		t.Fatal(e)
	}
	if e = worker.ProcessOnce(context.Background(), client, "machine"); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ac := f.config
	ac.BrokerURL = f.server.URL
	ac.DemoToken = "agent"
	ac.LedgerFile = filepath.Join(t.TempDir(), "ledger.json")
	stopped := make(chan error, 1)
	go func() { stopped <- agent.Run(ctx, ac, true) }()
	t.Cleanup(func() {
		cancel()
		if e := <-stopped; e != nil {
			t.Error(e)
		}
	})
	for n := 0; n < 100; n++ {
		var got core.Request
		f.call("alice", "GET", "/api/requests/"+r.Intent.ID, nil, 200, &got)
		if got.Status == "ready" {
			if got.ExitCode == nil || *got.ExitCode != 0 || got.Output == "" || got.Approvals[0].Kind != "ai" {
				t.Fatal("execution result invalid")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("model → reviewer signature → broker → actual agent did not finish")
}

func TestAIIdentityAndPolicyMisconfigurationDenied(t *testing.T) {
	f := modeFixture(t, "ai")
	f.call("machine", "GET", "/api/requests", nil, 403, nil)
	f.call("machine", "GET", "/api/catalog", nil, 403, nil)
	// Even possession of the AI service identity does not create a user session,
	// execute a command, alter membership or unlock an account.
	for _, path := range []string{"/api/requests", "/api/terminal/command", "/api/requests/anything/execute", "/api/account-locks/anything/unlock", "/api/team"} {
		f.call("machine", "POST", path, map[string]any{}, 403, nil)
	}
	f.call("bob", "GET", "/api/reviewer/queue", nil, 403, nil)
	for _, attack := range []string{"requester-role", "manual-ai", "no-human-fallback", "high-ai", "duplicate-key"} {
		var c core.Config
		json.Unmarshal(core.JSON(f.broker.Config), &c)
		switch attack {
		case "requester-role":
			c.Principals[len(c.Principals)-1].Roles = []string{"approver", "requester"}
		case "manual-ai":
			c.Targets[0].Actions[0].Review.Mode = "manual"
		case "no-human-fallback":
			c.Targets[0].Actions[0].Approvers = []string{"machine"}
		case "high-ai":
			c.Targets[0].Actions[0].Review.Risk = "high"
		case "duplicate-key":
			c.Principals[len(c.Principals)-1].PublicKey = c.Principals[1].PublicKey
		}
		if c.Validate(true) == nil {
			t.Fatalf("accepted unsafe config: %s", attack)
		}
	}
	keyPath := filepath.Join(t.TempDir(), "wrong-key")
	if e := os.WriteFile(keyPath, []byte(base64.StdEncoding.EncodeToString(f.keys["bob"])), 0600); e != nil {
		t.Fatal(e)
	}
	c := reviewer.Config{ID: "machine", BrokerURL: f.server.URL, DemoToken: "machine", PrivateKeyFile: keyPath, Simulation: true, MinConfidence: 0.9, Rules: []reviewer.Rule{{Target: "t1", Action: "status", CommandHash: f.broker.Config.Targets[0].Actions[0].CommandHash, Requesters: []string{"alice"}, Risk: "medium", MaxTTL: 300, MaxUses: 3}}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if reviewer.Run(ctx, c, true) == nil {
		t.Fatal("reviewer started with wrong signing key")
	}
}
