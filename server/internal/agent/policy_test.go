package agent

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"safeops/internal/core"
	"testing"
	"time"
)

func TestLocalModesAndSignedBudget(t *testing.T) {
	for _, attack := range []string{"valid-auto", "auto-high", "auto-unconfigured", "ai-for-manual", "spoof-kind", "mixed-quorum", "unsigned-budget", "lease-downgrade", "use-outside-budget", "too-many-uses"} {
		t.Run(attack, func(t *testing.T) {
			c, env, key := signedJob(t)
			rb := c.Runbooks["status"]
			i, _ := core.DecodeIntent(env.Lease.Payload)
			i.Version = 2
			i.MaxUses = 1
			env.Lease.Version = 2
			env.Lease.Use = 1
			rb.Review = core.ReviewPolicy{Mode: "auto", Risk: "low", MaxUses: 1}
			rb.Quorum = 0
			env.Lease.Approvals = nil
			switch attack {
			case "auto-high":
				rb.Review.Risk = "high"
			case "auto-unconfigured":
				rb.Review = core.ReviewPolicy{}
				rb.Quorum = 1
			case "ai-for-manual", "spoof-kind", "mixed-quorum", "unsigned-budget":
				pub, ak, _ := ed25519.GenerateKey(rand.Reader)
				c.ApproverKeys["machine"] = base64.StdEncoding.EncodeToString(pub)
				c.ApproverKinds = map[string]string{"machine": "ai"}
				rb.Approvers = []string{"machine", "bob"}
				rb.Quorum = 1
				rb.Review = core.ReviewPolicy{Mode: "manual", Risk: "medium", MaxUses: 3}
				env.Lease.Approvals = []core.Approval{{Subject: "machine", Kind: "human", Signature: core.Sign(ak, core.ApprovalDomain, core.Encode(i))}}
				if attack == "mixed-quorum" {
					rb.Review.Mode = "ai"
					rb.Quorum = 2
					env.Lease.Approvals = append(env.Lease.Approvals, core.Approval{Subject: "bob", Signature: "invalid"})
				}
				if attack == "unsigned-budget" {
					rb.Review.Mode = "ai"
					i.MaxUses = 2
				}
			case "lease-downgrade":
				env.Lease.Version = 1
				env.Lease.Use = 0
			case "use-outside-budget":
				env.Lease.Use = 2
			case "too-many-uses":
				i.MaxUses = 2
			}
			c.Runbooks["status"] = rb
			env.Lease.Payload = core.Encode(i)
			env.Signature = core.Sign(key, core.LeaseDomain, core.Encode(env.Lease))
			_, _, err := Verify(c, env, time.Now())
			if (err == nil) != (attack == "valid-auto") {
				t.Fatalf("unexpected verification: %v", err)
			}
		})
	}
}
func TestDurableUsesAndRateLimits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	// Legacy records must survive schema migration, including unusual IDs.
	if err := os.WriteFile(path, []byte(`{"old":true,"version":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	l, e := OpenLedger(path)
	if e != nil {
		t.Fatal(e)
	}
	i := core.Intent{Version: 2, ID: "grant", Target: "host", Action: "status", Requester: "alice", MaxUses: 3}
	rb := core.Runbook{Quorum: 1, Review: core.ReviewPolicy{Mode: "manual", Risk: "medium", MaxUses: 3, RateLimit: 2}}
	now := time.Now()
	if e = l.ConsumeGrant(i, core.Lease{Version: 2, Use: 1, ID: "lease1"}, rb, now); e != nil {
		t.Fatal(e)
	}
	l.Close()
	l, e = OpenLedger(path)
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	if l.Consume("old") == nil || l.Consume("version") == nil {
		t.Fatal("legacy consumption lost")
	}
	if l.ConsumeGrant(i, core.Lease{Version: 2, Use: 1, ID: "different-lease"}, rb, now) == nil {
		t.Fatal("lease ID bypassed use replay defense")
	}
	if e = l.ConsumeGrant(i, core.Lease{Version: 2, Use: 2}, rb, now); e != nil {
		t.Fatal(e)
	}
	if l.ConsumeGrant(i, core.Lease{Version: 2, Use: 3}, rb, now) == nil {
		t.Fatal("local rate not enforced")
	}
	if e = l.ConsumeGrant(i, core.Lease{Version: 2, Use: 3}, rb, now.Add(61*time.Second)); e != nil {
		t.Fatal(e)
	}
	if l.ConsumeGrant(i, core.Lease{Version: 2, Use: 4}, rb, now.Add(61*time.Second)) == nil {
		t.Fatal("use budget bypassed")
	}
}
func TestLedgerCorruptionFailsClosed(t *testing.T) {
	for _, raw := range []string{"null", "{}", "{", "{\"version\":2}"} {
		path := filepath.Join(t.TempDir(), "ledger.json")
		os.WriteFile(path, []byte(raw), 0600)
		l, e := OpenLedger(path)
		// An empty legacy map is valid. All other malformed states must be denied.
		if raw == "{}" && e == nil {
			l.Close()
			continue
		}
		if e == nil {
			l.Close()
			t.Fatalf("accepted %s", raw)
		}
	}
}
