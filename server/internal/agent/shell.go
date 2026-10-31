package agent

import (
	"context"
	"errors"
	"os"
	"runtime"
	"strconv"

	"safeops/internal/core"
)

func shellRunbook(p core.ShellPolicy, command string, demo bool) (core.Runbook, error) {
	if err := p.Validate(); err != nil {
		return core.Runbook{}, err
	}
	argv := []string{"/bin/sh", "-c", command}
	if p.Demo {
		if !demo || p.UID != uint32(os.Geteuid()) || p.GID != uint32(os.Getegid()) || os.Geteuid() == 0 {
			return core.Runbook{}, errors.New("local demo shell requires the current non-root demo account")
		}
	} else {
		if runtime.GOOS != "linux" || os.Geteuid() != 0 {
			return core.Runbook{}, errors.New("shell commands require the independent Linux execution service")
		}
		if !p.Privileged {
			// Drop all capabilities and supplementary groups before entering the shell.
			// Child commands cannot regain privileges through sudo/setuid executables.
			argv = append([]string{"/usr/bin/setpriv", "--no-new-privs", "--inh-caps=-all", "--ambient-caps=-all", "--bounding-set=-all", "--reuid=" + strconv.FormatUint(uint64(p.UID), 10), "--regid=" + strconv.FormatUint(uint64(p.GID), 10), "--clear-groups"}, argv...)
		}
	}
	return core.Runbook{Argv: argv, Timeout: p.Timeout}, nil
}

func executeShell(ctx context.Context, p core.ShellPolicy, command string, demo bool) (string, int) {
	rb, err := shellRunbook(p, command, demo)
	if err != nil {
		return err.Error(), -1
	}
	return executeInDirectory(ctx, rb, p.Directory)
}

func runnerPolicy(rb core.Runbook) bool {
	if rb.Diagnostics != nil && rb.Diagnostics.Shell != nil {
		return !rb.Diagnostics.Shell.Demo && (!rb.Diagnostics.Shell.Privileged || rb.Quorum >= 2)
	}
	return rb.Plan != nil && rb.Plan.Privileged && rb.Quorum >= 2
}
