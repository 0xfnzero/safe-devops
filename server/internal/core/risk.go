package core

const RiskDecisionDomain = "safeops.risk-decision.v1\n"
const UnlockDomain = "safeops.account-unlock.v1\n"

type AccountLock struct {
	ID            string `json:"id"`
	WorkspaceID   string `json:"workspace_id"`
	Requester     string `json:"requester"`
	ProjectID     string `json:"project_id"`
	Target        string `json:"target"`
	Action        string `json:"action"`
	SourceID      string `json:"source_id"`
	Reviewer      string `json:"reviewer"`
	Reason        string `json:"reason"`
	Model         string `json:"model"`
	ContextDigest string `json:"context_digest"`
	CreatedAt     int64  `json:"created_at"`
	ExpiresAt     int64  `json:"expires_at"`
	ReleasedAt    int64  `json:"released_at,omitempty"`
	ReleasedBy    string `json:"released_by,omitempty"`
}

func (l AccountLock) Active(now int64) bool { return l.ReleasedAt == 0 && now < l.ExpiresAt }

// Immutable fields only: unlock signatures cannot be replayed against a new lock.
func (l AccountLock) Payload() string { l.ReleasedAt = 0; l.ReleasedBy = ""; return Encode(l) }

type RiskScan struct {
	ID          string       `json:"id"`
	WorkspaceID string       `json:"workspace_id"`
	ProjectID   string       `json:"project_id"`
	Target      string       `json:"target"`
	Action      string       `json:"action"`
	CommandHash string       `json:"command_hash"`
	Requester   string       `json:"requester"`
	Cursor      string       `json:"cursor"`
	CreatedAt   int64        `json:"created_at"`
	ExpiresAt   int64        `json:"expires_at"`
	Context     *RiskContext `json:"risk_context"`
	Assessment  *Assessment  `json:"assessment,omitempty"`
}

func RiskDecisionPayload(id string, d ReviewSubmission) string {
	return Encode(struct {
		ID            string `json:"id"`
		ContextDigest string `json:"context_digest"`
		Decision      string `json:"decision"`
		Reason        string `json:"reason"`
		Model         string `json:"model"`
	}{id, d.ContextDigest, d.Decision, d.Reason, d.Model})
}
