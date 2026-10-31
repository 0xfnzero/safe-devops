package core

import (
	"errors"
	"slices"
)

// These policies are installed by administrators on both Broker and Agent.
// Request text and model output never select or relax a policy.
type ReviewPolicy struct {
	Mode      string `json:"mode"`
	Risk      string `json:"risk"`
	MaxUses   int    `json:"max_uses"`
	RateLimit int    `json:"rate_limit_per_minute"`
}
type Assessment struct {
	ContextDigest string `json:"context_digest,omitempty"`
	Reviewer      string `json:"reviewer"`
	Decision      string `json:"decision"`
	Reason        string `json:"reason"`
	Model         string `json:"model"`
	At            int64  `json:"at"`
}
type ReviewSubmission struct {
	ContextDigest string `json:"context_digest,omitempty"`
	Decision      string `json:"decision"`
	Reason        string `json:"reason"`
	Model         string `json:"model"`
	Signature     string `json:"signature,omitempty"`
}
type Execution struct {
	Operation  *DiagnosticOperation `json:"operation,omitempty"`
	Use        int                  `json:"use"`
	LeaseID    string               `json:"lease_id"`
	Status     string               `json:"status"`
	StartedAt  int64                `json:"started_at"`
	FinishedAt int64                `json:"finished_at,omitempty"`
	Output     string               `json:"output,omitempty"`
	ExitCode   *int                 `json:"exit_code,omitempty"`
}

func (p ReviewPolicy) Effective() ReviewPolicy {
	if p.Mode == "" {
		p.Mode = "manual"
	}
	if p.Risk == "" {
		p.Risk = "high"
	}
	if p.MaxUses == 0 {
		p.MaxUses = 1
	}
	if p.RateLimit == 0 {
		p.RateLimit = 6
	}
	return p
}
func Kind(kind string) string {
	if kind == "" {
		return "human"
	}
	return kind
}
func Uses(i Intent) int {
	if i.MaxUses == 0 {
		return 1
	}
	return i.MaxUses
}
func (p ReviewPolicy) Validate(quorum int) error {
	p = p.Effective()
	if !slices.Contains([]string{"auto", "ai", "manual"}, p.Mode) || !slices.Contains([]string{"low", "medium", "high"}, p.Risk) || p.MaxUses < 1 || p.MaxUses > 100 || p.RateLimit < 1 || p.RateLimit > 60 {
		return errors.New("invalid review policy")
	}
	if p.Risk == "high" && (p.Mode != "manual" || p.MaxUses != 1) {
		return errors.New("high risk requires human approval for each execution")
	}
	if p.Mode == "auto" {
		if p.Risk != "low" || quorum != 0 || p.MaxUses != 1 {
			return errors.New("automatic mode requires low risk, zero quorum and one use")
		}
	} else if quorum < 1 {
		return errors.New("reviewed tasks require a positive quorum")
	}
	return nil
}

// AI mode accepts either a full AI quorum or a full human fallback quorum.
// Partial human and machine approvals cannot be combined to bypass either.
func ApprovalSatisfied(policy ReviewPolicy, quorum int, payload, requester string, approvers []string, keys, kinds map[string]string, approvals []Approval, brokerKey string) bool {
	if policy.Validate(quorum) != nil {
		return false
	}
	p := policy.Effective()
	if p.Mode == "auto" {
		return true
	}
	seen := map[string]bool{}
	humans, machines := 0, 0
	for _, a := range approvals {
		key := keys[a.Subject]
		id := PublicKeyID(key)
		if a.Subject == requester || !slices.Contains(approvers, a.Subject) || id == "" || id == PublicKeyID(brokerKey) || seen[id] || !Verify(key, ApprovalDomain, payload, a.Signature) {
			continue
		}
		kind := Kind(kinds[a.Subject])
		if kind != "human" && kind != "ai" {
			continue
		}
		seen[id] = true
		if kind == "human" {
			humans++
		} else if p.Mode == "ai" {
			machines++
		}
	}
	return humans >= quorum || (p.Mode == "ai" && machines >= quorum)
}
