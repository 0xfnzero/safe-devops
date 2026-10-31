package agent

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"safeops/internal/core"
)

func TestDemoShellPipelineAndCleanEnvironment(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("demo refuses root")
	}
	t.Setenv("SAFEOPS_SECRET_TEST", "must-not-be-inherited")
	p := core.ShellPolicy{Directory: t.TempDir(), UID: uint32(os.Geteuid()), GID: uint32(os.Getegid()), Timeout: 3, Demo: true}
	out, code := executeShell(context.Background(), p, "printf 'hello' | tr a-z A-Z; printf '%s' \"$SAFEOPS_SECRET_TEST\"", true)
	if code != 0 || out != "HELLO" {
		t.Fatalf("%q %d", out, code)
	}
	if _, code := executeShell(context.Background(), p, "pwd", false); code == 0 {
		t.Fatal("production accepted demo shell")
	}
}

// CI runs this compiled test as root on a disposable Linux runner. It verifies
// actual UID/group/capability dropping and credential-file isolation.
func TestShellLinuxIsolation(t *testing.T) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		t.Skip("requires disposable Linux root runner")
	}
	dir, err := os.MkdirTemp("", "safe-devops-executor-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	if err = os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(dir, "work")
	if err = os.Mkdir(work, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.Chown(work, 65534, 65534); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(dir, "connector.key")
	if err = os.WriteFile(secret, []byte("PROTECTED-CREDENTIAL"), 0600); err != nil {
		t.Fatal(err)
	}
	p := core.ShellPolicy{Directory: work, UID: 65534, GID: 65534, Timeout: 5}
	out, code := executeShell(context.Background(), p, "id -u; id -G; grep NoNewPrivs /proc/self/status; printf allowed > result; cat result", false)
	if code != 0 || !strings.Contains(out, "65534\n65534\n") || !strings.Contains(out, "NoNewPrivs:\t1") || !strings.HasSuffix(out, "allowed") {
		t.Fatalf("isolation failed: %q %d", out, code)
	}
	out, code = executeShell(context.Background(), p, "cat '"+secret+"'", false)
	if code == 0 || strings.Contains(out, "PROTECTED-CREDENTIAL") {
		t.Fatal("executor read Connector credential")
	}
}
