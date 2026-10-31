package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"safeops/internal/core"
	"testing"
	"time"
)

func TestAtomicRollbackPersistenceAndAuditIntegrity(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.json")
	s, e := Open(ctx, path, "")
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Update(ctx, func(st *core.State) error {
		st.PolicyHash = "original"
		Append(st, time.Now(), "alice", "test", "r1", "original")
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	if e = s.Update(ctx, func(st *core.State) error { st.PolicyHash = "bad"; return errors.New("deny") }); e == nil {
		t.Fatal("failed update committed")
	}
	s.Close()
	s, e = Open(ctx, path, "")
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Update(ctx, func(st *core.State) error {
		if st.PolicyHash != "original" {
			t.Fatal("rollback failed")
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	s.Close()
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < len(b)-8; i++ {
		if string(b[i:i+8]) == "original" {
			b[i] = 'X'
		}
	}
	if e = os.WriteFile(path, b, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = Open(ctx, path, ""); e == nil {
		t.Fatal("corrupted audit accepted")
	}
}
