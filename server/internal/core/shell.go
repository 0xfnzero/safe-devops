package core

import (
	"errors"
	"path/filepath"
	"strings"
)

// ShellPolicy is installed on both Broker and executor. It authorizes submitting
// arbitrary shell text, not executing it without an exact, single-use approval.
// A working directory is a starting directory, NOT a filesystem sandbox.
type ShellPolicy struct {
	Directory  string `json:"directory"`
	UID        uint32 `json:"uid"`
	GID        uint32 `json:"gid"`
	Timeout    int    `json:"timeout_seconds"`
	Privileged bool   `json:"privileged,omitempty"`
	Demo       bool   `json:"demo,omitempty"`
}

func (p ShellPolicy) Validate() error {
	if p.UID == ^uint32(0) || p.GID == ^uint32(0) {
		return errors.New("invalid execution account ID")
	}
	if !filepath.IsAbs(p.Directory) || filepath.Clean(p.Directory) != p.Directory || !safeText(p.Directory, 1024) || p.Timeout < 1 || p.Timeout > 600 {
		return errors.New("shell requires an absolute working directory and 1–600 second timeout")
	}
	if !p.Demo && (p.Privileged != (p.UID == 0) || (!p.Privileged && p.GID == 0)) {
		return errors.New("shell must use a dedicated non-root UID/GID, or explicitly enable privileged execution")
	}
	return nil
}

func (d DiagnosticPolicy) Admit(line string) (DiagnosticOperation, error) {
	op, err := d.Parse(line)
	if d.Shell == nil {
		return op, err
	}
	if err == nil {
		words, _ := Words(line)
		// Explicit resource:path syntax keeps the scoped built-in reader. Ordinary
		// commands use real shell semantics and the visible starting directory.
		explicit := len(words) > 0 && op.Resource != "" && strings.HasPrefix(words[len(words)-1], op.Resource+":")
		if op.Tool == "status" || explicit {
			return op, nil
		}
	}
	op = DiagnosticOperation{Tool: "shell", Command: line}
	return op, d.ValidateOperation(op)
}

// This intentionally tiny fast path is NOT a command allowlist. Everything
// else remains submittable but requires human approval. Parsing shell text with
// regexes to declare it safe would allow substitutions, redirects and aliases
// to evade review. Only these exact strings may use automated review.
func ShellReview(command string) string {
	switch strings.TrimSpace(command) {
	case "ls", "ls -l", "ls -a", "ls -la", "ls -al", "pwd", "whoami", "uname", "uname -a", "uname -s", "uname -srm":
		return "auto"
	case "id", "uptime", "df -h", "free -h", "ps aux", "ps -ef":
		return "ai"
	default:
		return "manual"
	}
}

func (rb Runbook) ExecutionTimeout(i Intent) int {
	if rb.Diagnostics != nil && rb.Diagnostics.Shell != nil && i.Operation != nil && i.Operation.Tool == "shell" {
		return rb.Diagnostics.Shell.Timeout
	}
	return rb.Timeout
}
