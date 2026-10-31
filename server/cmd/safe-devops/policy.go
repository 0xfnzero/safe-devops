package main

import (
	"context"
	"fmt"
	"safeops/internal/core"
	"safeops/internal/store"
	"time"
)

func applyPolicy(ctx context.Context, st *store.Store, hash string, now time.Time) error {
	return st.Update(ctx, func(state *core.State) error {
		for _, req := range state.Requests {
			if req.Intent.ExpiresAt > now.Unix() {
				return fmt.Errorf("unexpired grants exist; retry after %s", time.Unix(req.Intent.ExpiresAt, 0).UTC().Format(time.RFC3339))
			}
		}
		for n := range state.Requests {
			r := &state.Requests[n]
			if r.Status == "pending" || r.Status == "approved" || r.Status == "ready" || r.Status == "running" {
				r.Status = "expired"
				r.Queued = false
			}
		}
		old := state.PolicyHash
		state.PolicyHash = hash
		store.Append(state, now, "administrator", "policy.applied", "", old+" -> "+hash)
		return nil
	})
}
