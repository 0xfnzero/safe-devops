package broker

import (
	"bytes"
	"io"
	"net/http"
	"slices"

	"safeops/internal/core"
)

// All clients use this admission point; the desktop cannot choose its own policy.
func (b *Broker) terminalCommand(w http.ResponseWriter, r *http.Request, p core.Principal) (any, error) {
	var in struct {
		Target  string `json:"target"`
		Profile string `json:"profile"`
		Command string `json:"command"`
		Reason  string `json:"reason"`
		Session string `json:"session,omitempty"`
	}
	if err := decode(w, r, &in); err != nil {
		return nil, err
	}
	if !core.HasRole(p, "requester") {
		return nil, bad(403, "requester role required")
	}
	target, profile, ok := b.Config.Action(in.Target, in.Profile)
	if !ok || !slices.Contains(target.Requesters, p.ID) {
		return nil, bad(403, "target/profile unavailable")
	}
	words, err := core.Words(in.Command)
	if err != nil {
		return nil, bad(400, err.Error())
	}
	body := map[string]any{"target": in.Target, "action": in.Profile, "max_uses": 1, "ttl_seconds": 900, "reason": in.Reason}
	if in.Session != "" {
		if profile.Diagnostics == nil {
			return nil, bad(400, "session requires diagnostic profile")
		}
		operation, err := profile.Diagnostics.Parse(in.Command)
		if err != nil {
			return nil, bad(400, err.Error())
		}
		// Validate the caller-selected target/profile against the immutable session.
		var valid bool
		if e := b.transaction(r.Context(), func(st *core.State) error {
			req, e := find(st, in.Session)
			if e != nil {
				return e
			}
			valid = req.Intent.Target == in.Target && req.Intent.Action == in.Profile
			return nil
		}); e != nil {
			return nil, e
		}
		if !valid {
			return nil, bad(403, "session belongs to a different target/profile")
		}
		r = r.Clone(r.Context())
		r.URL.Path = "/api/requests/" + in.Session + "/command"
		r.Body = io.NopCloser(bytes.NewReader(core.JSON(operation)))
		return b.requestAction(w, r, p)
	}
	switch words[0] {
	case "run":
		if len(words) != 2 {
			return nil, bad(400, "usage: run <installed-task-id>")
		}
		_, a, ok := b.Config.Action(in.Target, words[1])
		if !ok || a.Diagnostics != nil || a.Plan != nil {
			return nil, bad(400, "unknown fixed task")
		}
		body["action"] = a.ID
	case "plan":
		if len(words) < 3 {
			return nil, bad(400, "usage: plan <installed-plan-id> <step-id...>")
		}
		_, a, ok := b.Config.Action(in.Target, words[1])
		if !ok || a.Plan == nil {
			return nil, bad(400, "unknown change plan")
		}
		body["action"] = a.ID
		body["steps"] = words[2:]
	default:
		if profile.Diagnostics == nil {
			return nil, bad(400, "select a diagnostic profile")
		}
		operation, err := profile.Diagnostics.Parse(in.Command)
		if err != nil {
			return nil, bad(400, err.Error())
		}
		body["operation"] = operation
	}
	r = r.Clone(r.Context())
	r.Body = io.NopCloser(bytes.NewReader(core.JSON(body)))
	return b.createRequest(w, r, p)
}
