package agent

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"safeops/internal/core"
	"safeops/internal/device"
)

func Client(rawURL, caFile, certFile, keyFile string, demo bool) (*http.Client, error) {
	u, e := url.Parse(rawURL)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("invalid broker origin")
	}
	cfg := &tls.Config{MinVersion: tls.VersionTLS13}
	if demo {
		ip := net.ParseIP(u.Hostname())
		if u.Scheme != "http" || (u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback())) {
			return nil, errors.New("demo requires a loopback HTTP origin")
		}
	} else {
		if u.Scheme != "https" {
			return nil, errors.New("HTTPS required")
		}
		if caFile != "" {
			b, e := os.ReadFile(caFile)
			if e != nil {
				return nil, e
			}
			cfg.RootCAs = x509.NewCertPool()
			if !cfg.RootCAs.AppendCertsFromPEM(b) {
				return nil, errors.New("invalid CA")
			}
		}
		if certFile != "" || keyFile != "" {
			cert, e := tls.LoadX509KeyPair(certFile, keyFile)
			if e != nil {
				return nil, e
			}
			cfg.Certificates = []tls.Certificate{cert}
		}
	}
	client := &http.Client{Timeout: 8 * time.Second, Transport: &http.Transport{TLSClientConfig: cfg, Proxy: nil, MaxIdleConnsPerHost: 4}, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirects prohibited") }}
	if path := os.Getenv("SAFEOPS_DEVICE_KEY"); path != "" {
		if e := device.Enable(client, path); e != nil {
			return nil, e
		}
	}
	return client, nil
}
func Call(ctx context.Context, client *http.Client, base, token, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		body = bytes.NewReader(core.JSON(in))
	}
	r, e := http.NewRequestWithContext(ctx, method, strings.TrimRight(base, "/")+path, body)
	if e != nil {
		return e
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	r.Header.Set("Content-Type", "application/json")
	res, e := client.Do(r)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	b, e := io.ReadAll(io.LimitReader(res.Body, 1024*1024+1))
	if e != nil {
		return e
	}
	if len(b) > 1024*1024 {
		return errors.New("response too large")
	}
	if res.StatusCode != 200 {
		return fmt.Errorf("broker returned %d: %s", res.StatusCode, b)
	}
	if out != nil {
		return json.Unmarshal(b, out)
	}
	return nil
}

// Verification uses administrator-installed local policy, never review metadata
// supplied by the Broker. Automatic tasks deliberately require no reviewer key.
func Verify(c core.AgentConfig, e core.Envelope, now time.Time) (core.Intent, core.Runbook, error) {
	var empty core.Runbook
	l := e.Lease
	if !core.Verify(c.BrokerKey, core.LeaseDomain, core.Encode(l), e.Signature) {
		return core.Intent{}, empty, errors.New("invalid broker signature")
	}
	i, err := core.DecodeIntent(l.Payload)
	if err != nil {
		return i, empty, err
	}
	legacy := l.Version == 1 && i.Version == 1 && l.Use == 0 && i.MaxUses == 0 && i.WorkspaceID == "" && i.ProjectID == ""
	modern := l.Version == 2 && (i.Version == 2 && i.WorkspaceID == "" && i.ProjectID == "" || i.Version == 3 && core.ValidWorkspaceID(i.WorkspaceID) && core.ValidWorkspaceID(i.ProjectID)) && l.Use >= 1 && l.Use <= core.Uses(i) && i.MaxUses >= 1
	workflow := l.Version == 3 && i.Version == 4 && core.ValidWorkspaceID(i.WorkspaceID) && core.ValidWorkspaceID(i.ProjectID) && l.Use >= 1 && l.Use <= core.Uses(i) && i.MaxUses >= 1
	if (!legacy && !modern && !workflow) || l.ID == "" || i.ID == "" || l.AgentID != c.ID || i.Target != c.Target || i.WorkspaceID != c.WorkspaceID || i.ProjectID != c.ProjectID || now.Unix() >= l.ExpiresAt || now.Unix() >= i.ExpiresAt || l.ExpiresAt > i.ExpiresAt || i.CreatedAt > now.Unix()+5 || i.ExpiresAt <= i.CreatedAt || i.ExpiresAt-i.CreatedAt > core.MaxTTL {
		return i, empty, errors.New("invalid, expired or misrouted grant")
	}
	rb, ok := c.Runbooks[i.Action]
	if !ok || rb.ValidateSpec(false) != nil || rb.ValidateIntent(i) != nil || core.Uses(i) > rb.PolicyFor(i).MaxUses || !slices.Contains(rb.Requesters, i.Requester) || rb.Hash() != i.CommandHash {
		return i, empty, errors.New("local runbook policy denied execution")
	}
	if rb.Diagnostics != nil {
		if l.Operation == nil || rb.Diagnostics.ValidateOperation(*l.Operation) != nil {
			return i, empty, errors.New("invalid session operation")
		}
		if i.Kind == "session" && l.Operation.Tool == "shell" {
			return i, empty, errors.New("session grants never authorize shell operations")
		}
		if i.Kind == "command" && string(core.JSON(i.Operation)) != string(core.JSON(l.Operation)) {
			return i, empty, errors.New("command differs from approved intent")
		}
	} else if l.Operation != nil {
		return i, empty, errors.New("unexpected lease operation")
	}

	if !core.ApprovalSatisfied(rb.PolicyFor(i), rb.QuorumFor(i), l.Payload, i.Requester, rb.Approvers, c.ApproverKeys, c.ApproverKinds, l.Approvals, c.BrokerKey) {
		return i, empty, errors.New("local approval policy not satisfied")
	}
	return i, rb, nil
}

type cappedOutput struct {
	mu        sync.Mutex
	b         []byte
	truncated bool
}

func (o *cappedOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	n := len(p)
	left := core.MaxOutput - 64 - len(o.b)
	if len(p) > left {
		p = p[:max(0, left)]
		o.truncated = true
	}
	o.b = append(o.b, p...)
	return n, nil
}
func (o *cappedOutput) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	s := string(o.b)
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "")
	}
	if o.truncated {
		s += "\n[output truncated]"
	}
	return s
}
func Execute(ctx context.Context, rb core.Runbook) (string, int) {
	return executeInDirectory(ctx, rb, "/")
}
func executeInDirectory(ctx context.Context, rb core.Runbook, directory string) (string, int) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(rb.Timeout)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, rb.Argv[0], rb.Argv[1:]...)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LANG=C", "LC_ALL=C", "HOME=/nonexistent"}
	cmd.Dir = directory
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = 2 * time.Second
	out := &cappedOutput{}
	cmd.Stdout = out
	cmd.Stderr = out
	err := cmd.Run()
	if err == nil {
		return out.String(), 0
	}
	code := -1
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		code = exit.ExitCode()
	}
	return out.String() + "\n[execution failed or cancelled]", code
}
func Run(ctx context.Context, c core.AgentConfig, demo bool) error {
	if e := ValidateConfig(c, demo); e != nil {
		return e
	}
	if !demo && (c.TLSCert == "" || c.TLSKey == "") {
		return errors.New("agent mTLS certificate required")
	}
	client, e := Client(c.BrokerURL, c.CA, c.TLSCert, c.TLSKey, demo)
	if e != nil {
		return e
	}
	ledger, e := OpenLedger(c.LedgerFile)
	if e != nil {
		return e
	}
	defer ledger.Close()
	for {
		if ctx.Err() != nil {
			return nil
		}
		var poll struct {
			Job *core.Envelope `json:"job"`
		}
		if e = Call(ctx, client, c.BrokerURL, c.DemoToken, "POST", "/api/agent/poll", struct{}{}, &poll); e != nil {
			log.Printf("poll denied/unavailable: %v", e)
		} else if poll.Job != nil {
			process(ctx, client, c, ledger, *poll.Job)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(2 * time.Second):
		}
	}
}
func runVerified(ctx context.Context, client *http.Client, c core.AgentConfig, ledger *Ledger, env core.Envelope, privileged bool) (string, int, error) {
	i, rb, e := Verify(c, env, time.Now())
	if e != nil {
		return "", -1, e
	}
	if privileged && (!runnerPolicy(rb) || (rb.Diagnostics != nil && (i.Kind != "command" || i.Operation == nil || i.Operation.Tool != "shell"))) {
		return "", -1, errors.New("runner accepts exact reviewed shell commands or privileged two-human plans only")
	}
	input := map[string]any{"request_id": i.ID, "lease_id": env.Lease.ID}
	active := func(ctx context.Context) bool {
		var result struct {
			Active bool `json:"active"`
		}
		return Call(ctx, client, c.BrokerURL, c.DemoToken, "POST", "/api/agent/check", input, &result) == nil && result.Active
	}
	if !active(ctx) {
		return "", -1, errors.New("lease no longer active")
	}
	if e = ledger.ConsumeGrant(i, env.Lease, rb, time.Now()); e != nil {
		return "", -1, e
	}
	runCtx, cancel := context.WithDeadline(ctx, time.Unix(env.Lease.ExpiresAt, 0))
	done := make(chan struct{})
	monitorDone := make(chan struct{})
	go func() {
		defer close(monitorDone)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-runCtx.Done():
				return
			case <-ticker.C:
				checkCtx, stop := context.WithTimeout(runCtx, 1500*time.Millisecond)
				ok := active(checkCtx)
				stop()
				if !ok {
					cancel()
					return
				}
			}
		}
	}()
	var output string
	var code int
	if privileged && rb.Diagnostics != nil {
		output, code = executeShell(runCtx, *rb.Diagnostics.Shell, i.Operation.Command, false)
	} else if privileged {
		output, code = ExecutePlan(runCtx, i.Plan)
	} else {
		output, code = ExecuteWorkflow(runCtx, c, env, i, rb)
	}
	close(done)
	cancel()
	<-monitorDone
	return output, code, nil
}
func process(ctx context.Context, client *http.Client, c core.AgentConfig, ledger *Ledger, env core.Envelope) {
	output, code, err := runVerified(ctx, client, c, ledger, env, false)
	if err != nil {
		log.Printf("execution denied: %v", err)
		return
	}
	i, _ := core.DecodeIntent(env.Lease.Payload)
	input := map[string]any{"request_id": i.ID, "lease_id": env.Lease.ID}
	input["output"] = output
	input["exit_code"] = code
	resultCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 8*time.Second)
	defer stop()
	if e := Call(resultCtx, client, c.BrokerURL, c.DemoToken, "POST", "/api/agent/result", input, nil); e != nil {
		log.Printf("result delivery failed; command will NOT be retried: %v", e)
	}
}
