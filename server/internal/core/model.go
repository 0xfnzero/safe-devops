package core

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
)

const ApprovalDomain = "safeops.approval.v1\n"
const LeaseDomain = "safeops.lease.v1\n"
const MaxTTL = 900
const MaxOutput = 16384

type Principal struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Roles     []string `json:"roles"`
	TokenHash string   `json:"token_hash,omitempty"`
	PublicKey string   `json:"public_key,omitempty"`
	Kind      string   `json:"kind,omitempty"`
}
type AgentIdentity struct {
	ID        string `json:"id"`
	Target    string `json:"target"`
	TokenHash string `json:"token_hash,omitempty"`
}
type Action struct {
	Diagnostics *DiagnosticPolicy `json:"diagnostics,omitempty"`
	Plan        *PlanPolicy       `json:"plan,omitempty"`
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	CommandHash string            `json:"command_hash"`
	Approvers   []string          `json:"approvers"`
	Quorum      int               `json:"quorum"`
	Timeout     int               `json:"timeout_seconds"`
	Review      ReviewPolicy      `json:"review"`
}
type Target struct {
	ProjectID   string   `json:"project_id,omitempty"`
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Environment string   `json:"environment"`
	Requesters  []string `json:"requesters"`
	Actions     []Action `json:"actions"`
}
type Config struct {
	DevicePolicy    *DevicePolicy   `json:"device_policy,omitempty"`
	RiskLockSeconds int             `json:"risk_lock_seconds,omitempty"`
	Projects        []Project       `json:"projects,omitempty"`
	Database        DatabaseConfig  `json:"database"`
	Workspaces      []Workspace     `json:"workspaces,omitempty"`
	WorkspaceID     string          `json:"workspace_id,omitempty"`
	WorkspaceName   string          `json:"workspace_name,omitempty"`
	Listen          string          `json:"listen"`
	StateFile       string          `json:"state_file"`
	PrivateKeyFile  string          `json:"private_key_file"`
	TLSCert         string          `json:"tls_cert"`
	TLSKey          string          `json:"tls_key"`
	ClientCA        string          `json:"client_ca"`
	Principals      []Principal     `json:"principals"`
	Agents          []AgentIdentity `json:"agents"`
	Targets         []Target        `json:"targets"`
	DemoFile        string          `json:"demo_file,omitempty"`
}
type Intent struct {
	Operation   *DiagnosticOperation `json:"operation,omitempty"`
	Kind        string               `json:"kind,omitempty"`
	Scope       *DiagnosticPolicy    `json:"scope,omitempty"`
	Plan        []ChangeStep         `json:"plan,omitempty"`
	ProjectID   string               `json:"project_id,omitempty"`
	Version     int                  `json:"version"`
	WorkspaceID string               `json:"workspace_id,omitempty"`
	ID          string               `json:"id"`
	Target      string               `json:"target"`
	Action      string               `json:"action"`
	CommandHash string               `json:"command_hash"`
	Requester   string               `json:"requester"`
	Reason      string               `json:"reason"`
	CreatedAt   int64                `json:"created_at"`
	ExpiresAt   int64                `json:"expires_at"`
	MaxUses     int                  `json:"max_uses,omitempty"`
}
type Approval struct {
	Subject   string `json:"subject"`
	Signature string `json:"signature"`
	At        int64  `json:"at"`
	Kind      string `json:"kind,omitempty"`
}
type Request struct {
	DeviceID     string               `json:"device_id,omitempty"`
	Operation    *DiagnosticOperation `json:"operation,omitempty"`
	Context      *RiskContext         `json:"risk_context,omitempty"`
	Intent       Intent               `json:"intent"`
	Payload      string               `json:"payload"`
	Fingerprint  string               `json:"fingerprint"`
	Status       string               `json:"status"`
	Approvals    []Approval           `json:"approvals"`
	Quorum       int                  `json:"quorum"`
	LeaseID      string               `json:"lease_id,omitempty"`
	AgentID      string               `json:"agent_id,omitempty"`
	LeaseExpires int64                `json:"lease_expires,omitempty"`
	Output       string               `json:"output,omitempty"`
	ExitCode     *int                 `json:"exit_code,omitempty"`
	FinishedAt   int64                `json:"finished_at,omitempty"`
	Review       ReviewPolicy         `json:"review"`
	Used         int                  `json:"used"`
	Queued       bool                 `json:"queued"`
	NeedsHuman   bool                 `json:"needs_human"`
	Assessments  []Assessment         `json:"assessments"`
	Executions   []Execution          `json:"executions"`
}
type Lease struct {
	Operation *DiagnosticOperation `json:"operation,omitempty"`
	Version   int                  `json:"version"`
	ID        string               `json:"id"`
	AgentID   string               `json:"agent_id"`
	Payload   string               `json:"payload"`
	Approvals []Approval           `json:"approvals"`
	ExpiresAt int64                `json:"expires_at"`
	Use       int                  `json:"use,omitempty"`
}
type Envelope struct {
	Lease     Lease  `json:"lease"`
	Signature string `json:"signature"`
}
type Audit struct {
	Sequence  int    `json:"sequence"`
	At        int64  `json:"at"`
	Actor     string `json:"actor"`
	Event     string `json:"event"`
	RequestID string `json:"request_id"`
	Detail    string `json:"detail"`
	Previous  string `json:"previous"`
	Hash      string `json:"hash"`
}
type State struct {
	DeviceNonces []DeviceNonce    `json:"device_nonces,omitempty"`
	AccountLocks []AccountLock    `json:"account_locks,omitempty"`
	RiskScans    []RiskScan       `json:"risk_scans,omitempty"`
	PolicyHash   string           `json:"policy_hash"`
	Requests     []Request        `json:"requests"`
	Audit        []Audit          `json:"audit"`
	Heartbeats   map[string]int64 `json:"heartbeats"`
}
type Runbook struct {
	Diagnostics *DiagnosticPolicy `json:"diagnostics,omitempty"`
	Plan        *PlanPolicy       `json:"plan,omitempty"`
	Argv        []string          `json:"argv"`
	Timeout     int               `json:"timeout_seconds"`
	Approvers   []string          `json:"approvers"`
	Quorum      int               `json:"quorum"`
	Requesters  []string          `json:"requesters"`
	Review      ReviewPolicy      `json:"review"`
}
type AgentConfig struct {
	ChangeSocket  string             `json:"change_socket,omitempty"`
	ProjectID     string             `json:"project_id,omitempty"`
	WorkspaceID   string             `json:"workspace_id,omitempty"`
	ID            string             `json:"id"`
	Target        string             `json:"target"`
	BrokerURL     string             `json:"broker_url"`
	BrokerKey     string             `json:"broker_public_key"`
	ApproverKeys  map[string]string  `json:"approver_keys"`
	ApproverKinds map[string]string  `json:"approver_kinds,omitempty"`
	Runbooks      map[string]Runbook `json:"runbooks"`
	LedgerFile    string             `json:"ledger_file"`
	TLSCert       string             `json:"tls_cert"`
	TLSKey        string             `json:"tls_key"`
	CA            string             `json:"ca"`
	DemoToken     string             `json:"demo_token,omitempty"`
}

func ID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func Hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func JSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
func CommandHash(argv []string, timeout int) string {
	return Hash(JSON(struct {
		Argv    []string `json:"argv"`
		Timeout int      `json:"timeout_seconds"`
	}{argv, timeout}))
}
func Encode(v any) string { return base64.StdEncoding.EncodeToString(JSON(v)) }
func DecodeIntent(payload string) (Intent, error) {
	var i Intent
	b, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return i, err
	}
	err = json.Unmarshal(b, &i)
	if err == nil && Encode(i) != payload {
		err = errors.New("non-canonical intent")
	}
	return i, err
}
func Sign(key ed25519.PrivateKey, domain, payload string) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(key, []byte(domain+payload)))
}
func Verify(public, domain, payload, signature string) bool {
	key, e := base64.StdEncoding.DecodeString(public)
	if e != nil || len(key) != ed25519.PublicKeySize {
		return false
	}
	sig, e := base64.StdEncoding.DecodeString(signature)
	return e == nil && ed25519.Verify(key, []byte(domain+payload), sig)
}
func ReadKey(path string) (ed25519.PrivateKey, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	key, e := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	if e != nil || len(key) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid Ed25519 private key")
	}
	return ed25519.PrivateKey(key), nil
}
func HasRole(p Principal, role string) bool { return slices.Contains(p.Roles, role) }
func (c Config) Principal(id string) (Principal, bool) {
	for _, p := range c.Principals {
		if p.ID == id {
			return p, true
		}
	}
	return Principal{}, false
}
func (c Config) Action(target, action string) (Target, Action, bool) {
	for _, t := range c.Targets {
		if t.ID == target {
			for _, a := range t.Actions {
				if a.ID == action {
					return t, a, true
				}
			}
		}
	}
	return Target{}, Action{}, false
}
func ValidHash(h string) bool { b, e := hex.DecodeString(h); return e == nil && len(b) == 32 }
func PublicKeyID(public string) string {
	key, err := base64.StdEncoding.DecodeString(public)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return ""
	}
	return Hash(key)
}
func (c Config) Validate(demo bool) error {
	if err := c.DevicePolicy.Validate(c); err != nil {
		return err
	}
	if c.RiskLockSeconds != 0 && (c.RiskLockSeconds < 60 || c.RiskLockSeconds > 86400) {
		return errors.New("risk_lock_seconds must be 60..86400; default 900")
	}
	seen := map[string]bool{}
	tokens := map[string]bool{}
	approvalKeys := map[string]bool{}
	for _, p := range c.Principals {
		if !slices.Contains([]string{"human", "ai"}, Kind(p.Kind)) || p.ID == "" || seen[p.ID] || !ValidHash(p.TokenHash) || tokens[p.TokenHash] {
			return errors.New("invalid/duplicate principal or token")
		}
		seen[p.ID] = true
		tokens[p.TokenHash] = true
		if Kind(p.Kind) == "ai" && (len(p.Roles) != 1 || p.Roles[0] != "approver") {
			return errors.New("AI identities must have only the approver role")
		}
		if len(p.Roles) == 0 {
			return errors.New("principal requires roles")
		}
		for _, r := range p.Roles {
			if !slices.Contains([]string{"requester", "approver", "auditor"}, r) {
				return errors.New("unknown role")
			}
		}
		if HasRole(p, "approver") {
			k, e := base64.StdEncoding.DecodeString(p.PublicKey)
			if e != nil || len(k) != ed25519.PublicKeySize {
				return errors.New("approver requires Ed25519 public key")
			}
			canonical := base64.StdEncoding.EncodeToString(k)
			if approvalKeys[canonical] {
				return errors.New("independent approvers must use distinct keys")
			}
			approvalKeys[canonical] = true
		}
	}
	projects := map[string]Project{}
	for _, project := range c.Projects {
		if !ValidWorkspaceID(project.ID) || project.Name == "" || len(project.Name) > 100 {
			return errors.New("invalid project")
		}
		if _, exists := projects[project.ID]; exists {
			return errors.New("duplicate project")
		}
		members := map[string]bool{}
		for _, id := range project.Members {
			if _, ok := c.Principal(id); !ok || members[id] {
				return errors.New("invalid project member")
			}
			members[id] = true
		}
		projects[project.ID] = project
	}
	targets := map[string]bool{}
	for _, t := range c.Targets {
		if t.ID == "" || targets[t.ID] {
			return errors.New("duplicate/empty target")
		}
		project, projectExists := projects[t.ProjectID]
		if len(c.Projects) > 0 && !projectExists || len(c.Projects) == 0 && t.ProjectID != "" {
			return errors.New("server must belong to an existing project")
		}
		targets[t.ID] = true
		for _, id := range t.Requesters {
			p, ok := c.Principal(id)
			if !ok || !HasRole(p, "requester") || (projectExists && !slices.Contains(project.Members, id)) {
				return errors.New("invalid requester")
			}
		}
		acts := map[string]bool{}
		for _, a := range t.Actions {
			if a.ID == "" || acts[a.ID] || !ValidHash(a.CommandHash) || a.RunbookSpec().ValidateSpec(true) != nil {
				return errors.New("invalid action")
			}
			if !demo && a.Diagnostics != nil && a.Diagnostics.Shell != nil && a.Diagnostics.Shell.Demo {
				return errors.New("demo shell is forbidden in production")
			}
			if e := a.Review.Validate(a.Quorum); e != nil {
				return e
			}
			if (a.Diagnostics != nil || a.Plan != nil) && a.CommandHash != a.RunbookSpec().Hash() {
				return errors.New("workflow catalog hash does not match installed scope")
			}
			acts[a.ID] = true
			reviewers := map[string]bool{}
			for _, id := range a.Approvers {
				p, ok := c.Principal(id)
				if !ok || !HasRole(p, "approver") || reviewers[id] || (projectExists && !slices.Contains(project.Members, id)) {
					return errors.New("invalid approver list")
				}
				if a.Review.Effective().Mode == "auto" {
					return errors.New("automatic tasks must not list approvers")
				}
				if a.Review.Effective().Mode == "manual" && Kind(p.Kind) != "human" {
					return errors.New("manual tasks can only name human approvers")
				}
				reviewers[id] = true
			}
			for _, requester := range t.Requesters {
				humans, machines := 0, 0
				for id := range reviewers {
					if id == requester {
						continue
					}
					p, _ := c.Principal(id)
					if Kind(p.Kind) == "human" {
						humans++
					} else {
						machines++
					}
				}
				if a.Review.Effective().Mode != "auto" && humans < a.Quorum {
					return fmt.Errorf("unreachable independent human fallback for %s", a.ID)
				}
				if a.Review.Effective().Mode == "ai" && machines < a.Quorum {
					return fmt.Errorf("unreachable AI quorum for %s", a.ID)
				}
			}
		}
	}
	agents := map[string]bool{}
	assigned := map[string]bool{}
	for _, a := range c.Agents {
		if a.ID == "" || agents[a.ID] || assigned[a.Target] || !targets[a.Target] {
			return errors.New("invalid agent mapping; one agent per target required")
		}
		agents[a.ID] = true
		assigned[a.Target] = true
		if demo {
			if !ValidHash(a.TokenHash) || tokens[a.TokenHash] {
				return errors.New("invalid agent token")
			}
			tokens[a.TokenHash] = true
		}
	}
	if len(c.Principals) == 0 || len(c.Targets) == 0 || len(c.Agents) == 0 {
		return errors.New("principals, targets and agents required")
	}
	return nil
}
