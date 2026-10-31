package agent

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"

	"safeops/internal/core"
)

func ExecuteWorkflow(ctx context.Context, c core.AgentConfig, env core.Envelope, i core.Intent, rb core.Runbook) (string, int) {
	if rb.Diagnostics != nil {
		return Diagnose(ctx, *rb.Diagnostics, *env.Lease.Operation)
	}
	if rb.Plan != nil {
		if rb.Plan.Privileged {
			return callChangeRunner(ctx, c.ChangeSocket, env)
		}
		return ExecutePlan(ctx, i.Plan)
	}
	return Execute(ctx, rb)
}
func ExecutePlan(ctx context.Context, steps []core.ChangeStep) (string, int) {
	out := &cappedOutput{}
	for n, s := range steps {
		if ctx.Err() != nil {
			return out.String() + "\n[plan cancelled]", -1
		}
		fmt.Fprintf(out, "\n[%d/%d] %s\n", n+1, len(steps), s.Name)
		result, code := Execute(ctx, core.Runbook{Argv: s.Argv, Timeout: s.Timeout})
		fmt.Fprint(out, result)
		if code != 0 {
			return out.String(), code
		}
	}
	return out.String(), 0
}

// No shell, subprocess, path expansion or user-controlled executable is involved.
func Diagnose(ctx context.Context, p core.DiagnosticPolicy, o core.DiagnosticOperation) (string, int) {
	if err := p.ValidateOperation(o); err != nil {
		return err.Error(), 1
	}
	if ctx.Err() != nil {
		return "[cancelled]", -1
	}
	if o.Tool == "status" {
		host, _ := os.Hostname()
		return fmt.Sprintf("host: %s\nos: %s/%s\ntime: %s\n", host, runtime.GOOS, runtime.GOARCH, time.Now().UTC().Format(time.RFC3339)), 0
	}
	var base string
	for _, r := range p.Roots {
		if r.ID == o.Resource {
			base = r.Path
		}
	}
	root, err := os.OpenRoot(base)
	if err != nil {
		return "approved resource unavailable", 1
	}
	defer root.Close()
	// O_NONBLOCK prevents a malicious FIFO from hanging Open before fstat.
	f, err := root.OpenFile(o.Path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return "path unavailable or outside approved resource", 1
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "cannot inspect path", 1
	}
	out := &cappedOutput{}
	if o.Tool == "ls" {
		if !st.IsDir() {
			return "ls requires a directory", 1
		}
		entries, err := f.ReadDir(1001)
		if err != nil && err != io.EOF {
			return "directory unavailable", 1
		}
		for n, e := range entries {
			if ctx.Err() != nil {
				return out.String(), -1
			}
			if n == 1000 {
				fmt.Fprintln(out, "[directory listing truncated]")
				break
			}
			suffix := ""
			if e.IsDir() {
				suffix = "/"
			}
			fmt.Fprintln(out, e.Name()+suffix)
		}
		return out.String(), 0
	}
	if !st.Mode().IsRegular() {
		return "only regular files may be read", 1
	}
	const scanLimit int64 = 2 * 1024 * 1024
	var reader io.Reader = io.LimitReader(f, scanLimit)
	if o.Tool == "tail" && st.Size() > scanLimit {
		if _, err = f.Seek(-scanLimit, io.SeekEnd); err != nil {
			return "cannot read file tail", 1
		}
		br := bufio.NewReader(io.LimitReader(f, scanLimit))
		_, _ = br.ReadString('\n')
		reader = br
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 64*1024)
	tail := []string{}
	matched := false
	for scanner.Scan() {
		if ctx.Err() != nil {
			return out.String() + "\n[cancelled]", -1
		}
		line := scanner.Text()
		switch o.Tool {
		case "tail":
			tail = append(tail, line)
			if len(tail) > o.Lines {
				tail = tail[1:]
			}
		case "grep":
			if strings.Contains(line, o.Query) {
				matched = true
				fmt.Fprintln(out, line)
			}
		default:
			fmt.Fprintln(out, line)
		}
	}
	if o.Tool == "tail" {
		for _, line := range tail {
			fmt.Fprintln(out, line)
		}
	}
	if scanner.Err() != nil {
		return out.String() + "\n[line exceeds read limit or read failed]", 1
	}
	if st.Size() > scanLimit {
		fmt.Fprintln(out, "[scan limited to 2 MiB]")
	}
	if o.Tool == "grep" && !matched {
		return out.String(), 1
	}
	return out.String(), 0
}
