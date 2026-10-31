package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"safeops/internal/agent"
	"safeops/internal/broker"
	"safeops/internal/core"
	"safeops/internal/device"
	"safeops/internal/reviewer"
	"safeops/internal/store"
)

var version = "0.4.2-dev"

func main() {
	if e := run(); e != nil {
		log.Fatal(e)
	}
}
func load(path string, v any) error {
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return e
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("configuration must contain exactly one JSON document")
	}
	return nil
}
func run() error {
	if len(os.Args) < 2 {
		return errors.New("usage: safe-devops <init-compose|init-config|configure-shell|init-demo|broker|connector|change-runner|reviewer|approve|unlock-account|keygen|hash-runbook|check-config|backup|restore|policy-apply>")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	config := fs.String("config", "", "JSON configuration")
	demo := fs.Bool("demo", false, "unsafe local demonstration mode")
	switch os.Args[1] {
	case "init-compose", "init-config":
		dir := fs.String("dir", "", "new directory for configuration and private bootstrap files")
		workspace := fs.String("workspace", "", "team ID")
		project := fs.String("project", "", "initial project ID")
		origin := fs.String("broker-url", "", "HTTPS Broker origin")
		approval := fs.String("approver-public-key", "", "public key generated on independent human approval device")
		devicePublic := fs.String("device-public-key", "", "enrolled requester SSH Ed25519 public key (required)")
		diagnosticRoot := fs.String("diagnostic-root", "", "optional absolute directory on target server, excluding credentials")
		database := fs.String("database", "sqlite", "sqlite or postgres")
		webOrigin := fs.String("web-url", "", "HTTPS console origin for Compose")
		fs.Parse(os.Args[2:])
		if *dir == "" {
			return errors.New("--dir required")
		}
		if *devicePublic == "" {
			return errors.New("--device-public-key required; copy the device public key from the desktop launcher")
		}
		if os.Args[1] == "init-compose" {
			return initCompose(*dir, *workspace, *project, *origin, *webOrigin, *approval, *database, *diagnosticRoot, *devicePublic)
		}
		return initConfig(*dir, *workspace, *project, *origin, *approval, *database, *diagnosticRoot, *devicePublic)

	case "configure-shell":
		brokerPath := fs.String("broker-config", "", "existing Broker JSON")
		connectorPath := fs.String("connector-config", "", "existing Connector JSON")
		output := fs.String("out", "", "new output directory for coordinated policies")
		profile := fs.String("profile", "diagnostics", "existing diagnostic profile ID")
		directory := fs.String("directory", "", "initial command directory on target host (not a sandbox)")
		uid := fs.Uint("uid", 0, "dedicated execution account numeric UID")
		gid := fs.Uint("gid", 0, "dedicated execution account numeric GID")
		timeout := fs.Int("timeout", 120, "command timeout, 1–600 seconds")
		privileged := fs.Bool("privileged", false, "root commands; requires two independent human approvers")
		fs.Parse(os.Args[2:])
		if *output == "" || *uid > 4294967294 || *gid > 4294967294 {
			return errors.New("new --out and valid UID/GID required")
		}
		return configureShell(*brokerPath, *connectorPath, *output, *profile, core.ShellPolicy{Directory: *directory, UID: uint32(*uid), GID: uint32(*gid), Timeout: *timeout, Privileged: *privileged})
	case "init-demo":
		dir := fs.String("dir", ".safeops/v4", "new demo directory")
		fs.Parse(os.Args[2:])
		return initDemo(*dir)
	case "health":
		endpoint := fs.String("url", "https://localhost:8443/readyz", "HTTPS health endpoint")
		ca := fs.String("ca", "", "CA file")
		fs.Parse(os.Args[2:])
		u, e := url.Parse(*endpoint)
		if e != nil {
			return e
		}
		origin := *u
		origin.Path = ""
		origin.RawPath = ""
		client, e := agent.Client(origin.String(), *ca, "", "", false)
		if e != nil {
			return e
		}
		checkCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
		defer cancel()
		req, e := http.NewRequestWithContext(checkCtx, "GET", *endpoint, nil)
		if e != nil {
			return e
		}
		res, e := client.Do(req)
		if e != nil {
			return errors.New("health endpoint unavailable")
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			return errors.New("service not ready")
		}
		return nil
	case "version":
		fmt.Println(version)
		return nil
	case "device-public-key":
		key := fs.String("key", "", "Ed25519 private key file")
		fs.Parse(os.Args[2:])
		private, e := device.ReadKey(*key)
		if e != nil {
			return e
		}
		fmt.Println(device.Public(private))
		return nil
	case "device-headers":
		key := fs.String("key", "", "Ed25519 private key file")
		method := fs.String("method", "GET", "HTTP method")
		path := fs.String("path", "/api/me", "exact API path")
		bodyFile := fs.String("body-file", "", "exact request body file")
		tokenEnv := fs.String("token-env", "SAFEOPS_TOKEN", "identity credential environment variable")
		fs.Parse(os.Args[2:])
		private, e := device.ReadKey(*key)
		if e != nil {
			return e
		}
		var body []byte
		if *bodyFile != "" {
			body, e = os.ReadFile(*bodyFile)
			if e != nil {
				return e
			}
			if len(body) > 32768 {
				return errors.New("body too large")
			}
		}
		req, e := http.NewRequest(*method, "https://device.invalid"+*path, nil)
		if e != nil {
			return e
		}
		if raw := os.Getenv(*tokenEnv); raw != "" {
			req.Header.Set("Authorization", "Bearer "+raw)
		}
		device.SignHeaders(req, private, body, fmt.Sprint(time.Now().Unix()), core.ID())
		out := map[string]string{}
		for k, v := range req.Header {
			if strings.HasPrefix(k, "X-Safe-Devops-") {
				out[k] = v[0]
			}
		}
		return json.NewEncoder(os.Stdout).Encode(out)
	case "keygen":
		out := fs.String("out", "", "new private key file")
		fs.Parse(os.Args[2:])
		if *out == "" {
			return errors.New("--out required")
		}
		public, e := keyFile(*out)
		if e == nil {
			fmt.Println(public)
		}
		return e
	case "hash-runbook":
		action := fs.String("action", "", "runbook ID")
		fs.Parse(os.Args[2:])
		var c core.AgentConfig
		if e := load(*config, &c); e != nil {
			return e
		}
		rb, ok := c.Runbooks[*action]
		if !ok {
			return errors.New("unknown runbook")
		}
		fmt.Println(rb.Hash())
		return nil
	case "unlock-account":
		base := fs.String("url", "https://localhost:8443", "broker origin")
		id := fs.String("lock", "", "account lock ID")
		key := fs.String("key", "", "independent human private key")
		confirm := fs.String("confirm", "", "full lock fingerprint")
		ca := fs.String("ca", "", "CA file")
		tokenEnv := fs.String("token-env", "SAFEOPS_TOKEN", "identity token environment variable")
		fs.Parse(os.Args[2:])
		client, e := agent.Client(*base, *ca, "", "", *demo)
		if e != nil {
			return e
		}
		if os.Getenv("SAFEOPS_DEVICE_KEY") == "" && *key != "" {
			if e = device.Enable(client, *key); e != nil {
				return e
			}
		}
		var data struct {
			Locks []struct {
				Lock        core.AccountLock `json:"lock"`
				Payload     string           `json:"payload"`
				Fingerprint string           `json:"fingerprint"`
			} `json:"locks"`
		}
		if e = agent.Call(ctx, client, *base, os.Getenv(*tokenEnv), "GET", "/api/account-locks", nil, &data); e != nil {
			return e
		}
		for _, item := range data.Locks {
			if item.Lock.ID != *id {
				continue
			}
			l := item.Lock
			if l.Payload() != item.Payload || core.Hash(core.JSON(l)) != item.Fingerprint {
				return errors.New("lock payload mismatch")
			}
			pretty, _ := json.MarshalIndent(l, "", "  ")
			fmt.Printf("Review this account lock on an independent trusted device:\n%s\nFingerprint: %s\n", pretty, item.Fingerprint)
			if *confirm == "" {
				fmt.Println("To unlock, rerun with --confirm <fingerprint> and --key <private key>.")
				return nil
			}
			if *confirm != item.Fingerprint || !l.Active(time.Now().Unix()) {
				return errors.New("fingerprint mismatch or inactive lock")
			}
			private, e := core.ReadKey(*key)
			if e != nil {
				return e
			}
			d := map[string]string{"signature": core.Sign(private, core.UnlockDomain, item.Payload)}
			return agent.Call(ctx, client, *base, os.Getenv(*tokenEnv), "POST", "/api/account-locks/"+l.ID+"/unlock", d, nil)
		}
		return errors.New("lock not found")
	case "approve":
		base := fs.String("url", "https://localhost:8443", "broker origin")
		request := fs.String("request", "", "request ID")
		key := fs.String("key", "", "private key on independent approval device")
		confirm := fs.String("confirm", "", "exact SHA256 fingerprint after reviewing intent")
		ca := fs.String("ca", "", "CA file")
		tokenEnv := fs.String("token-env", "SAFEOPS_TOKEN", "environment variable containing identity token")
		fs.Parse(os.Args[2:])
		if *request == "" || strings.ContainsAny(*request, "/?#") {
			return errors.New("valid --request required")
		}
		client, e := agent.Client(*base, *ca, "", "", *demo)
		if e != nil {
			return e
		}
		if os.Getenv("SAFEOPS_DEVICE_KEY") == "" && *key != "" {
			if e = device.Enable(client, *key); e != nil {
				return e
			}
		}
		var req core.Request
		if e = agent.Call(ctx, client, *base, os.Getenv(*tokenEnv), "GET", "/api/requests/"+*request, nil, &req); e != nil {
			return e
		}
		intent, e := core.DecodeIntent(req.Payload)
		if e != nil {
			return e
		}
		fp := core.Hash(core.JSON(intent))
		if intent.ID != *request || core.Encode(req.Intent) != req.Payload || req.Fingerprint != fp {
			return errors.New("request payload mismatch")
		}
		pretty, _ := json.MarshalIndent(intent, "", "  ")
		fmt.Printf("Review this exact operation on this trusted device:\n%s\nFingerprint: %s\n", pretty, fp)
		if *confirm == "" {
			fmt.Println("To approve, rerun with --confirm <full fingerprint> and --key <private key>.")
			return nil
		}
		if *confirm != fp || time.Now().Unix() >= intent.ExpiresAt {
			return errors.New("fingerprint mismatch or expired request")
		}
		private, e := core.ReadKey(*key)
		if e != nil {
			return e
		}
		signature := core.Sign(private, core.ApprovalDomain, req.Payload)
		if e = agent.Call(ctx, client, *base, os.Getenv(*tokenEnv), "POST", "/api/requests/"+*request+"/approve", map[string]string{"signature": signature}, nil); e != nil {
			return e
		}
		fmt.Println("Approval submitted.")
		return nil
	case "broker":
		fs.Parse(os.Args[2:])
		var c core.Config
		if e := load(*config, &c); e != nil {
			return e
		}
		configs, e := validateBroker(c, *demo)
		if e != nil {
			return e
		}
		brokers := []*broker.Broker{}
		for _, cfg := range configs {
			opts, e := databaseOptions(c, *demo, cfg.WorkspaceID)
			if e != nil {
				return e
			}
			var st *store.Store
			if opts.Driver == "" {
				st, e = store.Open(ctx, c.StateFile, "")
			} else {
				st, e = store.OpenDatabase(ctx, opts)
			}
			if e != nil {
				return errors.New("database initialization failed; check connection, schema and filesystem permissions")
			}
			defer st.Close()
			key, e := core.ReadKey(cfg.PrivateKeyFile)
			if e != nil {
				return e
			}
			b, e := broker.New(ctx, cfg, st, key, *demo)
			if e != nil {
				return e
			}
			if *demo && c.DemoFile != "" {
				if e = load(c.DemoFile, &b.DemoData); e != nil {
					return e
				}
			}
			brokers = append(brokers, b)
		}
		router, e := broker.NewRouter(brokers, *demo)
		if e != nil {
			return e
		}
		if *demo {
			log.Print("DEMO: disposable human identity and signing credentials exposed on loopback only")
		}
		srv := &http.Server{Addr: c.Listen, Handler: router, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
		if !*demo {
			pem, e := os.ReadFile(c.ClientCA)
			if e != nil {
				return e
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(pem) {
				return errors.New("invalid client CA")
			}
			srv.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.VerifyClientCertIfGiven, ClientCAs: pool}
		}
		go func() {
			<-ctx.Done()
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = srv.Shutdown(shutdown)
		}()
		log.Printf("safe-devops broker listening on %s", c.Listen)
		if *demo {
			e = srv.ListenAndServe()
		} else {
			e = srv.ListenAndServeTLS(c.TLSCert, c.TLSKey)
		}
		if errors.Is(e, http.ErrServerClosed) {
			return nil
		}
		return e
	case "check-config":
		role := fs.String("role", "broker", "broker, connector or reviewer")
		fs.Parse(os.Args[2:])
		switch *role {
		case "broker":
			var c core.Config
			if e := load(*config, &c); e != nil {
				return e
			}
			if _, e := validateBroker(c, *demo); e != nil {
				return e
			}
		case "connector", "agent":
			var c core.AgentConfig
			if e := load(*config, &c); e != nil {
				return e
			}
			if e := agent.ValidateConfig(c, *demo); e != nil {
				return e
			}
		case "reviewer":
			var c reviewer.Config
			if e := load(*config, &c); e != nil {
				return e
			}
			if e := c.Validate(*demo); e != nil {
				return e
			}
			if _, e := agent.Client(c.BrokerURL, c.CA, "", "", *demo); e != nil {
				return e
			}
			if !*demo {
				if e := privateFile(c.PrivateKeyFile); e != nil {
					return e
				}
			}
			if _, e := core.ReadKey(c.PrivateKeyFile); e != nil {
				return e
			}
			if !c.Simulation {
				if _, e := reviewer.NewHTTPProvider(c, *demo); e != nil {
					return e
				}
			}
		default:
			return errors.New("unknown component role")
		}
		fmt.Println("Configuration valid; no service started and no database modified.")
		return nil
	case "policy-apply":
		workspace := fs.String("workspace", "", "workspace ID to update after all grants expire")
		fs.Parse(os.Args[2:])
		var c core.Config
		if e := load(*config, &c); e != nil {
			return e
		}
		configs, e := validateBroker(c, *demo)
		if e != nil {
			return e
		}
		for _, cfg := range configs {
			if cfg.WorkspaceID != *workspace {
				continue
			}
			opts, e := databaseOptions(c, *demo, cfg.WorkspaceID)
			if e != nil {
				return e
			}
			if opts.Driver == "" {
				return errors.New("policy-apply requires a database")
			}
			st, e := store.OpenDatabase(ctx, opts)
			if e != nil {
				return errors.New("database unavailable")
			}
			defer st.Close()
			key, e := core.ReadKey(cfg.PrivateKeyFile)
			if e != nil {
				return e
			}
			hash := broker.PolicyHash(cfg, key)
			e = applyPolicy(ctx, st, hash, time.Now())
			if e != nil {
				return e
			}
			fmt.Println("Workspace policy applied; restart Broker and install matching local Connector policy via the trusted management channel.")
			return nil
		}
		return errors.New("unknown --workspace")
	case "backup", "restore":
		workspace := fs.String("workspace", "", "workspace ID")
		out := fs.String("out", "", "new private backup file")
		input := fs.String("in", "", "backup file for restore into an empty workspace")
		fs.Parse(os.Args[2:])
		var c core.Config
		if e := load(*config, &c); e != nil {
			return e
		}
		configs, e := c.WorkspaceConfigs(*demo)
		if e != nil {
			return e
		}
		found := false
		for _, cfg := range configs {
			if cfg.WorkspaceID == *workspace {
				found = true
			}
		}
		if !found || !core.ValidWorkspaceID(*workspace) {
			return errors.New("choose an existing --workspace ID")
		}
		opts, e := databaseOptions(c, *demo, *workspace)
		if e != nil {
			return e
		}
		if opts.Driver == "" {
			return errors.New("backup/restore requires SQLite or PostgreSQL")
		}
		st, e := store.OpenDatabase(ctx, opts)
		if e != nil {
			return errors.New("database unavailable")
		}
		defer st.Close()
		if os.Args[1] == "backup" {
			if *out == "" {
				return errors.New("--out required")
			}
			snapshot, e := st.Backup(ctx)
			if e != nil {
				return e
			}
			f, e := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if e != nil {
				return e
			}
			_, e = f.Write(core.JSON(snapshot))
			if e == nil {
				e = f.Sync()
			}
			closeErr := f.Close()
			if e != nil {
				return e
			}
			if closeErr != nil {
				return closeErr
			}
			fmt.Println("Workspace backup saved. Protect this file; it contains operational data.")
			return nil
		}
		var snapshot store.Backup
		if e = load(*input, &snapshot); e != nil {
			return e
		}
		if e = st.Restore(ctx, snapshot); e != nil {
			return e
		}
		fmt.Println("Workspace restored into empty storage; active grants revoked. Preserve current Connector ledgers.")
		return nil
	case "change-runner":
		fs.Parse(os.Args[2:])
		if *demo {
			return errors.New("privileged runner has no demo mode")
		}
		if e := privateFile(*config); e != nil {
			return e
		}
		info, e := os.Stat(*config)
		if e != nil {
			return e
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != 0 {
			return errors.New("runner configuration must be root-owned")
		}
		var c core.AgentConfig
		if e := load(*config, &c); e != nil {
			return e
		}
		return agent.ChangeRunner(ctx, c)
	case "reviewer":
		fs.Parse(os.Args[2:])
		var c reviewer.Config
		if e := load(*config, &c); e != nil {
			return e
		}
		if !*demo {
			if e := privateFile(c.PrivateKeyFile); e != nil {
				return e
			}
		}
		return reviewer.Run(ctx, c, *demo)
	case "connector", "agent":
		fs.Parse(os.Args[2:])
		var c core.AgentConfig
		if e := load(*config, &c); e != nil {
			return e
		}
		log.Printf("safe-devops connector %s: outbound connection to %s", c.ID, c.BrokerURL)
		return agent.Run(ctx, c, *demo)
	default:
		return errors.New("unknown command")
	}
}
func keyFile(path string) (string, error) {
	pub, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		return "", e
	}
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return "", e
	}
	_, e = f.WriteString(base64.StdEncoding.EncodeToString(key))
	ce := f.Close()
	if e == nil {
		e = ce
	}
	return base64.StdEncoding.EncodeToString(pub), e
}
func writeJSON(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(path, append(b, '\n'), 0600)
}
func initDemo(dir string) error {
	dir, e := filepath.Abs(dir)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(dir), 0700); e != nil {
		return e
	}
	if e = os.Mkdir(dir, 0700); e != nil {
		return fmt.Errorf("demo directory must be new (existing credentials never overwritten): %w", e)
	}
	brokerKey, e := keyFile(filepath.Join(dir, "broker.key"))
	if e != nil {
		return e
	}
	principals := []core.Principal{}
	demoUsers := []map[string]string{}
	for _, u := range []struct{ id, name, role string }{{"alice", "Alice · 开发人员", "requester"}, {"bob", "Bob · 安全审批", "approver"}, {"carol", "Carol · 运维审批", "approver"}, {"auditor", "审计观察员", "auditor"}, {"dave", "Dave · 测试项目开发", "requester"}} {
		token := core.ID() + core.ID()
		p := core.Principal{ID: u.id, Name: u.name, Roles: []string{u.role}, TokenHash: core.Hash([]byte(token))}
		d := map[string]string{"id": u.id, "name": u.name, "role": u.role, "token": token}
		if u.role == "approver" {
			p.PublicKey, e = keyFile(filepath.Join(dir, u.id+".key"))
			if e != nil {
				return e
			}
			key, e := core.ReadKey(filepath.Join(dir, u.id+".key"))
			if e != nil {
				return e
			} // PKCS#8 seed encoding for WebCrypto demo signing only.
			d["seed"] = base64.StdEncoding.EncodeToString(key.Seed())
		}
		principals = append(principals, p)
		demoUsers = append(demoUsers, d)
	}
	aiToken := core.ID() + core.ID()
	aiKey, e := keyFile(filepath.Join(dir, "ai-reviewer.key"))
	if e != nil {
		return e
	}
	principals = append(principals, core.Principal{ID: "ai-reviewer", Name: "AI reviewer", Kind: "ai", Roles: []string{"approver"}, TokenHash: core.Hash([]byte(aiToken)), PublicKey: aiKey})
	runbooks := map[string]core.Runbook{
		"system-info":     {Argv: []string{"/usr/bin/uname", "-srm"}, Timeout: 10, Requesters: []string{"alice"}, Review: core.ReviewPolicy{Mode: "auto", Risk: "low", MaxUses: 1, RateLimit: 6}},
		"list-directory":  {Argv: []string{"/bin/ls", "-la", dir + "/diagnostics"}, Timeout: 10, Requesters: []string{"alice"}, Review: core.ReviewPolicy{Mode: "auto", Risk: "low", MaxUses: 1, RateLimit: 6}},
		"disk-usage":      {Argv: []string{"/bin/df", "-h"}, Timeout: 10, Approvers: []string{"bob", "carol", "ai-reviewer"}, Quorum: 1, Requesters: []string{"alice"}, Review: core.ReviewPolicy{Mode: "ai", Risk: "medium", MaxUses: 5, RateLimit: 6}},
		"host-uptime":     {Argv: []string{"/usr/bin/uptime"}, Timeout: 10, Approvers: []string{"bob", "carol"}, Quorum: 1, Requesters: []string{"alice"}, Review: core.ReviewPolicy{Mode: "manual", Risk: "medium", MaxUses: 5, RateLimit: 6}},
		"protected-check": {Argv: []string{"/usr/bin/uname", "-s"}, Timeout: 10, Approvers: []string{"bob", "carol"}, Quorum: 2, Requesters: []string{"alice"}, Review: core.ReviewPolicy{Mode: "manual", Risk: "high", MaxUses: 1, RateLimit: 6}},
	}
	runbooks["diagnostics"] = core.Runbook{Timeout: 15, Approvers: []string{"bob", "carol", "ai-reviewer"}, Quorum: 1, Requesters: []string{"alice"}, Review: core.ReviewPolicy{Mode: "ai", Risk: "medium", MaxUses: 100, RateLimit: 30}, Diagnostics: &core.DiagnosticPolicy{Shell: &core.ShellPolicy{Directory: filepath.Join(dir, "diagnostics"), UID: uint32(os.Geteuid()), GID: uint32(os.Getegid()), Timeout: 120, Demo: true}, Roots: []core.DiagnosticRoot{{ID: "logs", Path: filepath.Join(dir, "diagnostics")}}, Tools: []string{"ls", "cat", "tail", "grep", "status"}, AutoTools: []string{"ls", "status"}}}
	runbooks["demo-change"] = core.Runbook{Timeout: 30, Approvers: []string{"bob", "carol"}, Quorum: 2, Requesters: []string{"alice"}, Review: core.ReviewPolicy{Mode: "manual", Risk: "high", MaxUses: 1, RateLimit: 6}, Plan: &core.PlanPolicy{MaxSteps: 3, Steps: []core.ChangeStep{{ID: "check", Name: "检查环境 / Check environment", Description: "Harmless demonstration", Argv: []string{"/usr/bin/uname", "-srm"}, Timeout: 10}, {ID: "verify", Name: "验证运行状态 / Verify runtime", Description: "Harmless demonstration, no service modification", Argv: []string{"/usr/bin/uptime"}, Timeout: 10}}}}
	if e = os.Mkdir(filepath.Join(dir, "diagnostics"), 0700); e != nil {
		return e
	}
	if e = os.WriteFile(filepath.Join(dir, "diagnostics", "README.txt"), []byte("safe-devops diagnostic directory. No production secrets.\n"), 0600); e != nil {
		return e
	}
	if e = os.WriteFile(filepath.Join(dir, "diagnostics", "app.log"), []byte("2026-10-31 INFO service started\n2026-10-31 WARN upstream latency 250ms\n2026-10-31 ERROR upstream timeout\n2026-10-31 INFO retry completed\n"), 0600); e != nil {
		return e
	}
	actions := []core.Action{}
	for _, d := range []struct{ id, name, description string }{
		{"system-info", "系统信息", "低风险预授权：固定读取内核与架构"},
		{"list-directory", "列出演示目录", "固定 ls 路径，无自定义参数，无需逐次审批"},
		{"disk-usage", "磁盘容量检查", "AI 优先审核，可转人工；一次授权最多执行 5 次"},
		{"host-uptime", "运行时间检查", "人工授权窗口：同一任务最多执行 5 次"},
		{"diagnostics", "排障终端 / Diagnostic terminal", "常用诊断快速放行；支持通用命令，未知与变更操作转人工审核"},
		{"demo-change", "变更计划演示 / Change plan demo", "按选定顺序执行，双人批准；仅运行无害命令，不安装软件"},
		{"protected-check", "高风险策略演示", "双人逐次审批；演示仅运行无害的 uname，不修改系统"},
	} {
		rb := runbooks[d.id]
		actions = append(actions, core.Action{ID: d.id, Name: d.name, Description: d.description, Diagnostics: rb.Diagnostics, Plan: rb.Plan, CommandHash: rb.Hash(), Timeout: rb.Timeout, Approvers: rb.Approvers, Quorum: rb.Quorum, Review: rb.Review.Effective()})
	}
	rc := reviewer.Config{WorkspaceID: "demo-team", ID: "ai-reviewer", BrokerURL: "http://127.0.0.1:8088", DemoToken: aiToken, PrivateKeyFile: filepath.Join(dir, "ai-reviewer.key"), Simulation: true, MinConfidence: 0.9, Rules: []reviewer.Rule{{ProjectID: "payments", Target: "linux-demo", Action: "disk-usage", CommandHash: core.CommandHash(runbooks["disk-usage"].Argv, 10), Requesters: []string{"alice"}, Risk: "medium", MaxTTL: 900, MaxUses: 5}}}
	rc.Rules = append(rc.Rules, reviewer.Rule{ProjectID: "payments", Target: "linux-demo", Action: "diagnostics", CommandHash: runbooks["diagnostics"].Hash(), Requesters: []string{"alice"}, Risk: "medium", MaxTTL: 900, MaxUses: 100})
	if e = writeJSON(filepath.Join(dir, "reviewer.json"), rc); e != nil {
		return e
	}
	agentToken := core.ID() + core.ID()
	c := core.Config{WorkspaceID: "demo-team", WorkspaceName: "演示技术团队", Projects: []core.Project{{ID: "payments", Name: "支付平台", Description: "生产运维策略演示", Members: []string{"alice", "bob", "carol", "ai-reviewer"}}, {ID: "sandbox", Name: "测试平台", Description: "独立项目成员与服务器", Members: []string{"dave", "bob", "carol"}}}, Database: core.DatabaseConfig{Driver: "sqlite", SQLitePath: filepath.Join(dir, "safeops.db")}, Listen: "127.0.0.1:8088", PrivateKeyFile: filepath.Join(dir, "broker.key"), Principals: principals, Agents: []core.AgentIdentity{{ID: "agent-local", Target: "linux-demo", TokenHash: core.Hash([]byte(agentToken))}}, Targets: []core.Target{{ProjectID: "payments", ID: "linux-demo", Name: "Linux 演示节点", Environment: "DEMO", Requesters: []string{"alice"}, Actions: actions}}, DemoFile: filepath.Join(dir, "demo-users.json")}
	ac := core.AgentConfig{WorkspaceID: "demo-team", ProjectID: "payments", ID: "agent-local", Target: "linux-demo", BrokerURL: "http://127.0.0.1:8088", BrokerKey: brokerKey, ApproverKeys: map[string]string{}, ApproverKinds: map[string]string{}, Runbooks: runbooks, LedgerFile: filepath.Join(dir, "ledger.json"), DemoToken: agentToken}
	for _, p := range principals {
		if p.PublicKey != "" {
			ac.ApproverKeys[p.ID] = p.PublicKey
			ac.ApproverKinds[p.ID] = core.Kind(p.Kind)
		}
	}
	// Two projects with distinct membership; all demo Agents run locally.
	stagingToken := core.ID() + core.ID()
	c.Agents = append(c.Agents, core.AgentIdentity{ID: "agent-sandbox", Target: "sandbox-node", TokenHash: core.Hash([]byte(stagingToken))})
	c.Targets = append(c.Targets, core.Target{ProjectID: "sandbox", ID: "sandbox-node", Name: "测试平台演示节点", Environment: "DEMO", Requesters: []string{"dave"}, Actions: []core.Action{actions[0]}})
	secondary := ac
	secondary.ID = "agent-sandbox"
	secondary.Target = "sandbox-node"
	secondary.ProjectID = "sandbox"
	secondary.DemoToken = stagingToken
	secondary.LedgerFile = filepath.Join(dir, "sandbox-ledger.json")
	rb := runbooks["system-info"]
	rb.Requesters = []string{"dave"}
	secondary.Runbooks = map[string]core.Runbook{"system-info": rb}
	if e = writeJSON(filepath.Join(dir, "agent-sandbox.json"), secondary); e != nil {
		return e
	}
	if e = writeJSON(filepath.Join(dir, "broker.json"), c); e != nil {
		return e
	}
	if e = writeJSON(filepath.Join(dir, "agent.json"), ac); e != nil {
		return e
	}
	if e = writeJSON(c.DemoFile, map[string]any{"users": demoUsers}); e != nil {
		return e
	}
	fmt.Printf("Demo initialized at %s\nRun make demo, then visit http://127.0.0.1:3000\n", dir)
	return nil
}
