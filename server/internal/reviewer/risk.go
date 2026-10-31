package reviewer

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"safeops/internal/agent"
	"safeops/internal/core"
)

func (w *Worker) EvaluateRisk(ctx context.Context, scan core.RiskScan) core.ReviewSubmission {
	model := w.Config.Model
	if w.Config.Simulation {
		model = "demo-policy-simulator"
	}
	d := core.ReviewSubmission{Decision: "escalate", Reason: "Risk model unavailable or insufficient confidence; independent human investigation required.", Model: model, ContextDigest: scan.Context.Digest()}
	var rule *Rule
	if scan.WorkspaceID == w.Config.WorkspaceID && scan.ExpiresAt > w.Now().Unix() && scan.CreatedAt <= w.Now().Unix()+5 && scan.Context != nil {
		for n := range w.Config.Rules {
			r := &w.Config.Rules[n]
			if r.ProjectID == scan.ProjectID && r.Target == scan.Target && r.Action == scan.Action && r.CommandHash == scan.CommandHash && slices.Contains(r.Requesters, scan.Requester) {
				rule = r
				break
			}
		}
	}
	if rule != nil {
		if w.Config.Simulation {
			d.Decision = "approve"
			d.Reason = "LOCAL SIMULATION: no configured risk trigger found; no external AI called."
			failures := 0
			for _, v := range scan.Context.Records {
				if v.ExitCode != nil && *v.ExitCode != 0 {
					failures++
				}
			}
			// Explicitly a simulator, not a production security classifier.
			if failures >= 5 {
				d.Decision = "lock"
				d.Reason = "LOCAL SIMULATION: five recent failed operations triggered temporary account containment."
			}
		} else if w.Provider != nil {
			i := core.Intent{Version: 4, Kind: "risk_scan", ID: scan.ID, WorkspaceID: scan.WorkspaceID, ProjectID: scan.ProjectID, Target: scan.Target, Action: scan.Action, CommandHash: scan.CommandHash, Requester: scan.Requester, Reason: "Assess recent activity for account compromise. This is a retrospective scan; no command is being authorized.", CreatedAt: scan.CreatedAt, ExpiresAt: scan.ExpiresAt, MaxUses: 1}
			decision, err := w.Provider.Review(ctx, i, *rule, scan.Context)
			if err == nil && validDecision(decision) && *decision.Confidence >= w.Config.MinConfidence {
				d.Decision = decision.Decision
				d.Reason = decision.Reason
			}
		}
	}
	if ctx.Err() != nil || scan.ExpiresAt <= w.Now().Unix() {
		d.Decision = "escalate"
		d.Reason = "Risk review expired or was cancelled; human investigation required."
	}
	d.Signature = core.Sign(w.Key, core.RiskDecisionDomain, core.RiskDecisionPayload(scan.ID, d))
	return d
}
func (w *Worker) ProcessRiskOnce(ctx context.Context, client *http.Client, token string) error {
	var queue struct {
		Scans []core.RiskScan `json:"scans"`
	}
	if err := agent.Call(ctx, client, w.Config.BrokerURL, token, "GET", "/api/reviewer/risk-queue", nil, &queue); err != nil {
		return err
	}
	for _, scan := range queue.Scans {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if scan.ID == "" || strings.ContainsAny(scan.ID, "/?#") {
			continue
		}
		// Scans outside this worker's installed rules are left for the assigned worker.
		eligible := false
		for _, r := range w.Config.Rules {
			if scan.WorkspaceID == w.Config.WorkspaceID && scan.ProjectID == r.ProjectID && scan.Target == r.Target && scan.Action == r.Action && scan.CommandHash == r.CommandHash && slices.Contains(r.Requesters, scan.Requester) {
				eligible = true
			}
		}
		if !eligible {
			continue
		}
		d := w.EvaluateRisk(ctx, scan)
		if err := agent.Call(ctx, client, w.Config.BrokerURL, token, "POST", "/api/reviewer/risk-scans/"+scan.ID, d, nil); err != nil {
			return err
		}
	}
	return nil
}
