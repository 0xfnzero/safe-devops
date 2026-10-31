package main

import (
	"context"
	"path/filepath"
	"safeops/internal/core"
	"safeops/internal/store"
	"testing"
	"time"
)

func TestPolicyApplyWaitsForExpiryAndPreservesAudit(t *testing.T) {
	ctx := context.Background()
	st, e := store.OpenDatabase(ctx, store.Options{Driver: "sqlite", Path: filepath.Join(t.TempDir(), "state.db"), Workspace: "acme"})
	if e != nil {
		t.Fatal(e)
	}
	defer st.Close()
	now := time.Now()
	e = st.Update(ctx, func(s *core.State) error {
		s.PolicyHash = "old"
		s.Requests = []core.Request{{Status: "ready", Queued: true, Intent: core.Intent{ID: "r", ExpiresAt: now.Unix() + 10}}}
		store.Append(s, now, "test", "initial", "", "")
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	if applyPolicy(ctx, st, "new", now) == nil {
		t.Fatal("accepted unexpired grant")
	}
	before, _ := st.Backup(ctx)
	if before.State.PolicyHash != "old" || len(before.State.Audit) != 1 {
		t.Fatal("failed change modified state")
	}
	if e = applyPolicy(ctx, st, "new", now.Add(11*time.Second)); e != nil {
		t.Fatal(e)
	}
	after, e := st.Backup(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if after.State.PolicyHash != "new" || len(after.State.Audit) != 2 || after.State.Requests[0].Status != "expired" || after.State.Requests[0].Queued {
		t.Fatal("policy change lost history or retained grant")
	}
}
