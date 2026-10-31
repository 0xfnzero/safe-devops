package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"safeops/internal/core"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPostgresWorkspaceConcurrencyAndPortableBackup(t *testing.T) {
	dsn := os.Getenv("SAFEOPS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires dedicated *_test PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, e := sql.Open("pgx", dsn)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	var name string
	if e = db.QueryRowContext(ctx, "SELECT current_database()").Scan(&name); e != nil {
		t.Fatal(e)
	}
	if !strings.HasSuffix(name, "_test") {
		t.Fatal("test database suffix required")
	}
	workspace := "pg-" + core.ID()
	otherID := "pg-" + core.ID()
	t.Cleanup(func() { db.Exec("DELETE FROM safeops_workspaces WHERE id IN ($1,$2)", workspace, otherID) })
	open := func(id string) *Store {
		s, e := OpenDatabase(ctx, Options{Driver: "postgres", URL: dsn, Workspace: id})
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { s.Close() })
		return s
	}
	a, b, other := open(workspace), open(workspace), open(otherID)
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
				st.Heartbeats["counter"]++
				Append(st, time.Now(), "test", "increment", "", "")
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
	snapshot, e := a.Backup(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if snapshot.State.Heartbeats["counter"] != 20 || len(snapshot.State.Audit) != 20 {
		t.Fatal("lost update")
	}
	foreign, e := other.Backup(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if len(foreign.State.Audit) != 0 {
		t.Fatal("tenant leak")
	}
	if other.Restore(ctx, snapshot) == nil {
		t.Fatal("foreign backup accepted")
	}
	sqlite, e := OpenDatabase(ctx, Options{Driver: "sqlite", Path: filepath.Join(t.TempDir(), "restore.db"), Workspace: workspace})
	if e != nil {
		t.Fatal(e)
	}
	defer sqlite.Close()
	if e = sqlite.Restore(ctx, snapshot); e != nil {
		t.Fatal(e)
	}
	result, e := sqlite.Backup(ctx)
	if e != nil || len(result.State.Audit) != 21 {
		t.Fatal("portable restore failed", e)
	}
	// A tampered backup is rejected before writing anything.
	snapshot.State.PolicyHash = "tampered"
	if e = sqlite.Restore(ctx, snapshot); e == nil || !strings.Contains(e.Error(), "digest") {
		t.Fatal("corrupt backup not rejected by digest")
	}
}
