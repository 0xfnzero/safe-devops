package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"safeops/internal/core"
)

type runnerResult struct {
	Output   string `json:"output"`
	ExitCode int    `json:"exit_code"`
}

func callChangeRunner(ctx context.Context, socket string, env core.Envelope) (string, int) {
	if !filepath.IsAbs(socket) {
		return "privileged executor unavailable", -1
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "unix", socket)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect denied") }}
	req, err := http.NewRequestWithContext(ctx, "POST", "http://runner/execute", bytes.NewReader(core.JSON(env)))
	if err != nil {
		return "invalid runner request", -1
	}
	res, err := client.Do(req)
	if err != nil {
		return "privileged executor unavailable or cancelled", -1
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 128*1024))
	var out runnerResult
	if err != nil || res.StatusCode != 200 || json.Unmarshal(data, &out) != nil || len(out.Output) > core.MaxOutput {
		return "privileged executor denied execution", -1
	}
	return out.Output, out.ExitCode
}
func RunnerHandler(c core.AgentConfig, client *http.Client, ledger *Ledger) http.Handler {
	slot := make(chan struct{}, 1)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/execute" {
			http.Error(w, "not found", 404)
			return
		}
		select {
		case slot <- struct{}{}:
			defer func() { <-slot }()
		default:
			http.Error(w, "runner busy", 409)
			return
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128*1024))
		decoder.DisallowUnknownFields()
		var env core.Envelope
		if decoder.Decode(&env) != nil || decoder.Decode(new(any)) != io.EOF {
			http.Error(w, "invalid envelope", 400)
			return
		}
		output, code, err := runVerified(r.Context(), client, c, ledger, env, true)
		if err != nil {
			http.Error(w, "signed plan, approval, active lease or replay validation failed", 403)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(runnerResult{output, code})
	})
}

// ChangeRunner is intentionally a separate root-owned, Unix-only service.
func ChangeRunner(ctx context.Context, c core.AgentConfig) error {
	if os.Geteuid() != 0 {
		return errors.New("change-runner requires the dedicated root service")
	}
	if err := ValidateConfig(c, false); err != nil {
		return err
	}
	for _, rb := range c.Runbooks {
		if !runnerPolicy(rb) {
			return errors.New("runner config must contain only reviewed shell profiles or privileged plans")
		}
	}
	dir := filepath.Dir(c.ChangeSocket)
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 || !info.IsDir() || info.Mode().Perm()&0027 != 0 {
		return errors.New("runner socket directory must be root-owned, group-readable and not group-writable (0750)")
	}
	client, err := Client(c.BrokerURL, c.CA, c.TLSCert, c.TLSKey, false)
	if err != nil {
		return err
	}
	ledger, err := OpenLedger(c.LedgerFile)
	if err != nil {
		return err
	}
	defer ledger.Close()
	listener, err := net.Listen("unix", c.ChangeSocket)
	if err != nil {
		return err
	}
	defer listener.Close()
	if err = os.Chmod(c.ChangeSocket, 0660); err != nil {
		return err
	}
	if err = os.Chown(c.ChangeSocket, 0, int(stat.Gid)); err != nil {
		return err
	}
	server := &http.Server{Handler: RunnerHandler(c, client, ledger), ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 610 * time.Second, IdleTimeout: 10 * time.Second, MaxHeaderBytes: 4096, BaseContext: func(net.Listener) context.Context { return ctx }}
	go func() { <-ctx.Done(); _ = server.Close() }()
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
