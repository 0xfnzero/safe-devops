package agent

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"safeops/internal/core"
)

func signedJob(t *testing.T) (core.AgentConfig, core.Envelope, ed25519.PrivateKey) {
	t.Helper()
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	apub, akey, _ := ed25519.GenerateKey(rand.Reader)
	rb := core.Runbook{Argv: []string{"/usr/bin/uname", "-s"}, Timeout: 5, Quorum: 1, Approvers: []string{"bob"}, Requesters: []string{"alice"}}
	c := core.AgentConfig{ID: "a1", Target: "t1", BrokerKey: base64.StdEncoding.EncodeToString(pub), ApproverKeys: map[string]string{"bob": base64.StdEncoding.EncodeToString(apub)}, Runbooks: map[string]core.Runbook{"status": rb}}
	i := core.Intent{Version: 1, ID: "r1", Target: "t1", Action: "status", CommandHash: core.CommandHash(rb.Argv, rb.Timeout), Requester: "alice", Reason: "inspect the host", CreatedAt: time.Now().Unix(), ExpiresAt: time.Now().Unix() + 300}
	l := core.Lease{Version: 1, ID: "l1", AgentID: "a1", Payload: core.Encode(i), ExpiresAt: time.Now().Unix() + 10, Approvals: []core.Approval{{Subject: "bob", Signature: core.Sign(akey, core.ApprovalDomain, core.Encode(i))}}}
	return c, core.Envelope{Lease: l, Signature: core.Sign(key, core.LeaseDomain, core.Encode(l))}, key
}
func TestCompromisedBrokerCannotBypassLocalApproval(t *testing.T) {
	for _, attack := range []string{"remove-approval", "alter-intent", "different-target", "different-agent", "expired", "local-command-drift", "increase-quorum", "duplicate-approver", "self-approve", "unallowed-requester", "extend-ttl"} {
		t.Run(attack, func(t *testing.T) {
			c, e, key := signedJob(t)
			i, _ := core.DecodeIntent(e.Lease.Payload)
			switch attack {
			case "remove-approval":
				e.Lease.Approvals = nil
			case "alter-intent":
				i.Reason = "malicious intent"
				e.Lease.Payload = core.Encode(i)
			case "different-target":
				c.Target = "other"
			case "different-agent":
				c.ID = "other"
			case "expired":
				e.Lease.ExpiresAt = time.Now().Unix() - 1
			case "local-command-drift":
				rb := c.Runbooks["status"]
				rb.Argv = []string{"/bin/echo", "different"}
				c.Runbooks["status"] = rb
			case "increase-quorum", "duplicate-approver":
				rb := c.Runbooks["status"]
				rb.Quorum = 2
				c.Runbooks["status"] = rb
				if attack == "duplicate-approver" {
					e.Lease.Approvals = append(e.Lease.Approvals, e.Lease.Approvals[0])
				}
			case "self-approve":
				i.Requester = "bob"
				e.Lease.Payload = core.Encode(i)
			case "unallowed-requester":
				rb := c.Runbooks["status"]
				rb.Requesters = []string{"carol"}
				c.Runbooks["status"] = rb
			case "extend-ttl":
				i.ExpiresAt = i.CreatedAt + 901
				e.Lease.Payload = core.Encode(i)
			}
			e.Signature = core.Sign(key, core.LeaseDomain, core.Encode(e.Lease))
			if _, _, err := Verify(c, e, time.Now()); err == nil {
				t.Fatal("attack passed local verifier")
			}
		})
	}
}
func TestValidGrantAndDurableReplayDenial(t *testing.T) {
	c, e, _ := signedJob(t)
	intent, _, err := Verify(c, e, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "ledger.json")
	l, err := OpenLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = l.Consume(intent.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = OpenLedger(path); err == nil {
		t.Fatal("two agents acquired same ledger")
	}
	l.Close()
	l, err = OpenLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if err = l.Consume(intent.ID); err == nil {
		t.Fatal("restart allowed replay")
	}
}
func TestExecutionIsBoundedAndDoesNotInheritSecrets(t *testing.T) {
	t.Setenv("SAFEOPS_TEST_SECRET", "never-output-this")
	out, code := Execute(context.Background(), core.Runbook{Argv: []string{"/usr/bin/env"}, Timeout: 5})
	if code != 0 || strings.Contains(out, "SAFEOPS_TEST_SECRET") {
		t.Fatalf("environment leaked: code=%d output=%s", code, out)
	}
	start := time.Now()
	_, code = Execute(context.Background(), core.Runbook{Argv: []string{"/bin/sleep", "5"}, Timeout: 1})
	if code == 0 || time.Since(start) > 4*time.Second {
		t.Fatal("command deadline not enforced")
	}
}
func TestTransportRejectsPlaintextAndRedirects(t *testing.T) {
	for _, u := range []string{"http://example.com", "https://user:pass@example.com", "https://example.com/path"} {
		if _, e := Client(u, "", "", "", false); e == nil {
			t.Fatalf("accepted unsafe URL %s", u)
		}
	}
	if _, e := Client("http://example.com", "", "", "", true); e == nil {
		t.Fatal("demo allowed remote HTTP")
	}
}

func TestSameKeyCannotSatisfyTwoApprovers(t *testing.T) {
	c, env, brokerKey := signedJob(t)
	rb := c.Runbooks["status"]
	rb.Quorum = 2
	rb.Approvers = []string{"bob", "carol"}
	c.Runbooks["status"] = rb
	// Base64 decoders permit line breaks: compare actual key identity.
	c.ApproverKeys["carol"] = c.ApproverKeys["bob"][:10] + "\n" + c.ApproverKeys["bob"][10:]
	duplicate := env.Lease.Approvals[0]
	duplicate.Subject = "carol"
	env.Lease.Approvals = append(env.Lease.Approvals, duplicate)
	env.Signature = core.Sign(brokerKey, core.LeaseDomain, core.Encode(env.Lease))
	if _, _, e := Verify(c, env, time.Now()); e == nil {
		t.Fatal("one key counted as two independent reviewers")
	}
}
