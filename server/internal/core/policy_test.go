package core

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

func TestReviewPolicyGuardrails(t *testing.T) {
	for _, tc := range []struct {
		p     ReviewPolicy
		q     int
		valid bool
	}{
		{ReviewPolicy{}, 1, true}, {ReviewPolicy{Mode: "auto", Risk: "low"}, 0, true},
		{ReviewPolicy{Mode: "auto", Risk: "high"}, 0, false}, {ReviewPolicy{Mode: "auto", Risk: "low", MaxUses: 2}, 0, false},
		{ReviewPolicy{Mode: "ai", Risk: "high"}, 1, false}, {ReviewPolicy{Mode: "manual", Risk: "high", MaxUses: 2}, 1, false},
		{ReviewPolicy{Mode: "ai", Risk: "medium", MaxUses: 5}, 1, true}, {ReviewPolicy{Mode: "manual", Risk: "medium", MaxUses: 101}, 1, false},
		{ReviewPolicy{Mode: "manual", Risk: "medium"}, 0, false}, {ReviewPolicy{Mode: "auto", Risk: "low"}, 1, false},
	} {
		if (tc.p.Validate(tc.q) == nil) != tc.valid {
			t.Fatalf("policy validation: %+v", tc)
		}
	}
}
func TestHumanAndAIQuorumsCannotBeCombined(t *testing.T) {
	keys, kinds := map[string]string{}, map[string]string{}
	approvals := []Approval{}
	for _, id := range []string{"h1", "h2", "ai1", "ai2"} {
		pub, key, _ := ed25519.GenerateKey(rand.Reader)
		keys[id] = base64.StdEncoding.EncodeToString(pub)
		kinds[id] = "human"
		if id[:2] == "ai" {
			kinds[id] = "ai"
		}
		approvals = append(approvals, Approval{Subject: id, Kind: "human", Signature: Sign(key, ApprovalDomain, "payload")})
	}
	p := ReviewPolicy{Mode: "ai", Risk: "medium"}
	ids := []string{"h1", "h2", "ai1", "ai2"}
	satisfied := func(a []Approval) bool { return ApprovalSatisfied(p, 2, "payload", "alice", ids, keys, kinds, a, "") }
	if satisfied([]Approval{approvals[0], approvals[2]}) {
		t.Fatal("partial quorums combined")
	}
	if !satisfied(approvals[:2]) || !satisfied(approvals[2:]) {
		t.Fatal("full quorum denied")
	}
	p.Mode = "manual"
	if satisfied(approvals[2:]) {
		t.Fatal("AI metadata forged human quorum")
	}
}
