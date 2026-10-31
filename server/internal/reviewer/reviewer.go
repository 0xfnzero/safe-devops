// Package reviewer runs on a separate trusted host. It can sign fixed intents,
// never execute commands or change Broker/Agent policy.
package reviewer

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"log"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"safeops/internal/agent"
	"safeops/internal/core"
)

type Rule struct {
	ProjectID   string   `json:"project_id,omitempty"`
	Target      string   `json:"target"`
	Action      string   `json:"action"`
	CommandHash string   `json:"command_hash"`
	Requesters  []string `json:"requesters"`
	Risk        string   `json:"risk"`
	MaxTTL      int      `json:"max_ttl_seconds"`
	MaxUses     int      `json:"max_uses"`
}
type Config struct {
	WorkspaceID    string  `json:"workspace_id,omitempty"`
	ID             string  `json:"id"`
	BrokerURL      string  `json:"broker_url"`
	CA             string  `json:"ca"`
	TokenEnv       string  `json:"token_env"`
	DemoToken      string  `json:"demo_token,omitempty"`
	PrivateKeyFile string  `json:"private_key_file"`
	Endpoint       string  `json:"endpoint"`
	Model          string  `json:"model"`
	APIKeyEnv      string  `json:"api_key_env"`
	MinConfidence  float64 `json:"min_confidence"`
	Rules          []Rule  `json:"rules"`
	Simulation     bool    `json:"simulation"`
}
type Decision struct {
	Decision   string   `json:"decision"`
	Reason     string   `json:"reason"`
	Confidence *float64 `json:"confidence"`
}
type Provider interface {
	Review(context.Context, core.Intent, Rule, *core.RiskContext) (Decision, error)
}
type Worker struct {
	Config   Config
	Key      ed25519.PrivateKey
	Provider Provider
	Now      func() time.Time
}

func (c Config) Validate(demo bool) error {
	if c.ID == "" || c.PrivateKeyFile == "" || len(c.Rules) == 0 || c.MinConfidence < 0.5 || c.MinConfidence > 1 {
		return errors.New("invalid reviewer identity, rules or confidence threshold")
	}
	if c.Simulation && !demo {
		return errors.New("simulation requires --demo")
	}
	if !demo && (c.TokenEnv == "" || c.DemoToken != "" || !core.ValidWorkspaceID(c.WorkspaceID)) {
		return errors.New("production reviewer requires token_env and forbids demo_token")
	}
	for _, r := range c.Rules {
		if !demo && !core.ValidWorkspaceID(r.ProjectID) {
			return errors.New("production reviewer rules require project_id")
		}
		if r.Target == "" || r.Action == "" || !core.ValidHash(r.CommandHash) || len(r.Requesters) == 0 || !slices.Contains([]string{"low", "medium"}, r.Risk) || r.MaxTTL < 30 || r.MaxTTL > core.MaxTTL || r.MaxUses < 1 || r.MaxUses > 100 {
			return errors.New("invalid reviewer allowlist rule")
		}
	}
	return nil
}
func (w *Worker) allowed(req core.Request) (core.Intent, Rule, bool) {
	i, e := core.DecodeIntent(req.Payload)
	if e != nil || (i.Version != 2 && i.Version != 3 && i.Version != 4) || i.WorkspaceID != w.Config.WorkspaceID || (i.Version == 2 && (i.WorkspaceID != "" || i.ProjectID != "")) || (i.Version >= 3 && (!core.ValidWorkspaceID(i.WorkspaceID) || !core.ValidWorkspaceID(i.ProjectID))) || core.Encode(req.Intent) != req.Payload || req.Fingerprint != core.Hash(core.JSON(i)) || i.ID == "" || strings.ContainsAny(i.ID, "/?#") || req.Status != "pending" || req.NeedsHuman || req.Review.Mode != "ai" || i.CreatedAt > w.Now().Unix()+5 || i.ExpiresAt <= w.Now().Unix() || i.ExpiresAt <= i.CreatedAt {
		return i, Rule{}, false
	}
	if i.Version == 4 && (i.Kind != "command" && i.Kind != "session" || i.Scope == nil || len(i.Plan) > 0 || i.Scope.Validate() != nil) {
		return i, Rule{}, false
	}
	if i.Version == 4 {
		if i.Kind == "command" && (i.Operation == nil || i.MaxUses != 1 || i.Scope.ValidateOperation(*i.Operation) != nil) {
			return i, Rule{}, false
		}
		if i.Kind == "session" && i.Operation != nil {
			return i, Rule{}, false
		}
	}
	for _, r := range w.Config.Rules {
		if r.ProjectID == i.ProjectID && r.Target == i.Target && r.Action == i.Action && r.CommandHash == i.CommandHash && slices.Contains(r.Requesters, i.Requester) && r.Risk == req.Review.Risk && slices.Contains([]string{"low", "medium"}, r.Risk) && i.ExpiresAt-i.CreatedAt <= int64(r.MaxTTL) && i.MaxUses >= 1 && i.MaxUses <= r.MaxUses {
			return i, r, true
		}
	}
	return i, Rule{}, false
}
func (w *Worker) Evaluate(ctx context.Context, req core.Request) core.ReviewSubmission {
	model := w.Config.Model
	if w.Config.Simulation {
		model = "demo-policy-simulator"
	}
	fallback := core.ReviewSubmission{Decision: "escalate", Reason: "Independent policy denied review or model unavailable; human review required.", Model: model, ContextDigest: req.Context.Digest()}
	i, rule, ok := w.allowed(req)
	if !ok {
		return fallback
	}
	var d Decision
	var e error
	if w.Config.Simulation {
		confidence := 1.0
		d = Decision{Decision: "approve", Reason: "LOCAL SIMULATION: fixed allowlisted diagnostic; no external AI model was called.", Confidence: &confidence}
		if strings.Contains(strings.ToLower(i.Reason), "[human]") {
			d.Decision = "escalate"
			d.Reason = "LOCAL SIMULATION: explicit human review requested."
		}
		if req.Context != nil {
			failures := 0
			for _, v := range req.Context.Records {
				if v.ExitCode != nil && *v.ExitCode != 0 {
					failures++
				}
			}
			if failures >= 3 {
				d.Decision = "escalate"
				d.Reason = "LOCAL SIMULATION: repeated recent failures require human investigation."
			}
		}
	} else if w.Provider == nil {
		return fallback
	} else {
		d, e = w.Provider.Review(ctx, i, rule, req.Context)
	}
	if e != nil || !validDecision(d) {
		return fallback
	}
	if (d.Decision == "approve" || d.Decision == "lock") && *d.Confidence < w.Config.MinConfidence {
		fallback.Reason = "Model confidence below administrator threshold; human review required."
		return fallback
	}
	// Expiration is rechecked after the remote call. A late answer never signs.
	if _, _, ok = w.allowed(req); !ok || ctx.Err() != nil {
		return fallback
	}
	out := core.ReviewSubmission{Decision: d.Decision, Reason: d.Reason, Model: model, ContextDigest: req.Context.Digest()}
	if d.Decision == "approve" {
		out.Signature = core.Sign(w.Key, core.ApprovalDomain, req.Payload)
	}
	if d.Decision == "lock" {
		out.Signature = core.Sign(w.Key, core.RiskDecisionDomain, core.RiskDecisionPayload(req.Intent.ID, out))
	}
	return out
}
func validDecision(d Decision) bool {
	return slices.Contains([]string{"approve", "escalate", "reject", "lock"}, d.Decision) && len(strings.TrimSpace(d.Reason)) >= 3 && len(d.Reason) <= 2000 && !strings.ContainsAny(d.Reason, "\x00\x1b") && d.Confidence != nil && *d.Confidence >= 0 && *d.Confidence <= 1
}
func (w *Worker) ProcessOnce(ctx context.Context, client *http.Client, token string) error {
	var queue struct {
		Requests []core.Request `json:"requests"`
	}
	if e := agent.Call(ctx, client, w.Config.BrokerURL, token, "GET", "/api/reviewer/queue", nil, &queue); e != nil {
		return e
	}
	for _, req := range queue.Requests {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if req.Intent.ID == "" || strings.ContainsAny(req.Intent.ID, "/?#") {
			continue
		}
		decision := w.Evaluate(ctx, req)
		if e := agent.Call(ctx, client, w.Config.BrokerURL, token, "POST", "/api/requests/"+req.Intent.ID+"/review", decision, nil); e != nil {
			return e
		}
	}
	return nil
}
func Run(ctx context.Context, c Config, demo bool) error {
	if e := c.Validate(demo); e != nil {
		return e
	}
	client, e := agent.Client(c.BrokerURL, c.CA, "", "", demo)
	if e != nil {
		return e
	}
	key, e := core.ReadKey(c.PrivateKeyFile)
	if e != nil {
		return e
	}
	token := os.Getenv(c.TokenEnv)
	if demo && c.DemoToken != "" {
		token = c.DemoToken
	}
	if token == "" {
		return errors.New("reviewer token environment variable is empty")
	}
	var identity struct {
		Principal core.Principal `json:"principal"`
		Workspace struct {
			ID string `json:"id"`
		} `json:"workspace"`
	}
	if e = agent.Call(ctx, client, c.BrokerURL, token, "GET", "/api/me", nil, &identity); e != nil {
		return e
	}
	me := identity.Principal
	public := base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey))
	if identity.Workspace.ID != c.WorkspaceID || me.ID != c.ID || core.Kind(me.Kind) != "ai" || !core.HasRole(me, "approver") || core.PublicKeyID(me.PublicKey) != core.PublicKeyID(public) {
		return errors.New("reviewer identity/signing key mismatch")
	}
	w := &Worker{Config: c, Key: key, Now: time.Now}
	if !c.Simulation {
		w.Provider, e = NewHTTPProvider(c, demo)
		if e != nil {
			return e
		}
	}
	if c.Simulation {
		log.Print("AI reviewer DEMO SIMULATION: no external model called")
	}
	for {
		if e = w.ProcessOnce(ctx, client, token); e != nil && ctx.Err() == nil {
			log.Print("reviewer poll/submission unavailable; requests remain pending for human handoff")
		}
		if e = w.ProcessRiskOnce(ctx, client, token); e != nil && ctx.Err() == nil {
			log.Print("risk scan unavailable; no automatic lock applied")
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(2 * time.Second):
		}
	}
}
