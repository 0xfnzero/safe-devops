package store

import (
	"context"
	"errors"
	"path/filepath"
	"safeops/internal/core"
	"sync"
	"testing"
	"time"
)

func TestSQLiteConcurrencyIsolationBackupRestore(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "control.db")
	open := func(workspace string) *Store {
		s, e := OpenDatabase(ctx, Options{Driver: "sqlite", Path: path, Workspace: workspace})
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { s.Close() })
		return s
	}
	a, b, other := open("alpha"), open("alpha"), open("beta")
	var journal string
	if e := a.db.QueryRow(`PRAGMA journal_mode`).Scan(&journal); e != nil || journal != "wal" {
		t.Fatalf("WAL unavailable: %s %v", journal, e)
	}
	var syncMode int
	if e := a.db.QueryRow(`PRAGMA synchronous`).Scan(&syncMode); e != nil || syncMode != 2 {
		t.Fatal("SQLite durability is not FULL")
	}
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for n := 0; n < 20; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			s := a
			if n%2 == 1 {
				s = b
			}
			errs <- s.Update(ctx, func(st *core.State) error {
				st.Heartbeats["count"]++
				Append(st, time.Now(), "user", "increment", "", "")
				return nil
			})
		}(n)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	if e := a.Update(ctx, func(st *core.State) error {
		if st.Heartbeats["count"] != 20 || len(st.Audit) != 20 {
			t.Fatal("lost concurrent update")
		}
		st.PolicyHash = "should-rollback"
		return errors.New("deny")
	}); e == nil {
		t.Fatal("missing rollback error")
	}
	snapshot, e := a.Backup(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if snapshot.State.PolicyHash != "" {
		t.Fatal("rollback failed")
	}
	foreign, e := other.Backup(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if len(foreign.State.Audit) != 0 || foreign.State.Heartbeats["count"] != 0 {
		t.Fatal("cross-workspace leak")
	}
	if other.Restore(ctx, snapshot) == nil {
		t.Fatal("cross-workspace restore accepted")
	}
	restored, e := OpenDatabase(ctx, Options{Driver: "sqlite", Path: filepath.Join(t.TempDir(), "restore.db"), Workspace: "alpha"})
	if e != nil {
		t.Fatal(e)
	}
	defer restored.Close()
	snapshot.State.Requests = append(snapshot.State.Requests, core.Request{Status: "ready", Queued: true, Intent: core.Intent{ID: "old-grant", ExpiresAt: time.Now().Unix() + 300}})
	snapshot.Digest = core.Hash(core.JSON(snapshot.State))
	if e = restored.Restore(ctx, snapshot); e != nil {
		t.Fatal(e)
	}
	result, e := restored.Backup(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if result.State.Requests[0].Status != "revoked" || result.State.Requests[0].Queued || len(result.State.Audit) != 21 {
		t.Fatal("restore retained live grant or lost audit")
	}
	if restored.Restore(ctx, snapshot) == nil {
		t.Fatal("restore overwrote existing workspace")
	}
}
func TestSQLiteRestartAndUnknownSchema(t *testing.T) {
	ctx := context.Background()
	o := Options{Driver: "sqlite", Path: filepath.Join(t.TempDir(), "control.db"), Workspace: "alpha"}
	s, e := OpenDatabase(ctx, o)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Update(ctx, func(st *core.State) error { st.PolicyHash = "persisted"; return nil }); e != nil {
		t.Fatal(e)
	}
	s.Close()
	s, e = OpenDatabase(ctx, o)
	if e != nil {
		t.Fatal(e)
	}
	snapshot, e := s.Backup(ctx)
	if e != nil || snapshot.State.PolicyHash != "persisted" {
		t.Fatal("restart lost state")
	}
	s.db.Exec(`UPDATE safeops_schema SET version=999 WHERE id=1`)
	s.Close()
	if s, e = OpenDatabase(ctx, o); e == nil {
		s.Close()
		t.Fatal("future schema accepted")
	}
}
