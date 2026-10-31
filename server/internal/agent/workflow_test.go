package agent

import (
	"context"
	"os"
	"path/filepath"
	"safeops/internal/core"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestDiagnosticsConfinementAndSpecialFiles(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "logs")
	if e := os.Mkdir(root, 0700); e != nil {
		t.Fatal(e)
	}
	_ = os.WriteFile(filepath.Join(dir, "secret"), []byte("MUST_NOT_LEAK"), 0600)
	_ = os.WriteFile(filepath.Join(root, "app.log"), []byte("first\nERROR second\nlast\n"), 0600)
	if e := os.Symlink(filepath.Join(dir, "secret"), filepath.Join(root, "escape")); e != nil {
		t.Fatal(e)
	}
	if e := syscall.Mkfifo(filepath.Join(root, "fifo"), 0600); e != nil {
		t.Fatal(e)
	}
	p := core.DiagnosticPolicy{Roots: []core.DiagnosticRoot{{ID: "logs", Path: root}}, Tools: []string{"ls", "cat", "tail", "grep", "status"}}
	for _, name := range []string{"escape", "fifo", "../secret"} {
		start := time.Now()
		out, code := Diagnose(context.Background(), p, core.DiagnosticOperation{Tool: "cat", Resource: "logs", Path: name})
		if code == 0 || strings.Contains(out, "MUST_NOT_LEAK") || time.Since(start) > time.Second {
			t.Fatalf("unsafe read: %s %d %s", name, code, out)
		}
	}
	for _, tc := range []struct {
		op   core.DiagnosticOperation
		want string
	}{{core.DiagnosticOperation{Tool: "tail", Resource: "logs", Path: "app.log", Lines: 1}, "last\n"}, {core.DiagnosticOperation{Tool: "grep", Resource: "logs", Path: "app.log", Query: "ERROR"}, "ERROR second\n"}} {
		out, code := Diagnose(context.Background(), p, tc.op)
		if code != 0 || out != tc.want {
			t.Fatalf("%d %q", code, out)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, code := Diagnose(ctx, p, core.DiagnosticOperation{Tool: "status"}); code == 0 {
		t.Fatal("cancelled diagnostic executed")
	}
}
func TestPlanStopsAfterFailure(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "must-not-exist")
	out, code := ExecutePlan(context.Background(), []core.ChangeStep{{ID: "fail", Name: "fail", Argv: []string{"/usr/bin/false"}, Timeout: 5}, {ID: "write", Name: "write", Argv: []string{"/usr/bin/touch", marker}, Timeout: 5}})
	if code == 0 || !strings.Contains(out, "fail") {
		t.Fatal("failure lost")
	}
	if _, e := os.Stat(marker); !os.IsNotExist(e) {
		t.Fatal("plan continued after failure")
	}
}
