package main

import (
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"safeops/internal/core"
	"safeops/internal/store"
)

func privateFile(path string) error {
	info, e := os.Lstat(path)
	if e != nil {
		return e
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("private file must be regular and 0600: %s", path)
	}
	return nil
}
func databaseOptions(c core.Config, demo bool, workspace string) (store.Options, error) {
	if workspace == "" {
		workspace = "default"
	}
	db := c.Database
	if db.Driver == "" {
		if demo {
			return store.Options{}, nil
		}
		return store.Options{}, errors.New("configure database.driver as sqlite or postgres")
	}
	o := store.Options{Driver: db.Driver, Path: db.SQLitePath, Workspace: workspace}
	switch db.Driver {
	case "sqlite":
		if !filepath.IsAbs(db.SQLitePath) || db.URLEnv != "" {
			return o, errors.New("SQLite requires an absolute sqlite_path and no url_env")
		}
	case "postgres":
		if db.SQLitePath != "" || db.URLEnv == "" {
			return o, errors.New("PostgreSQL requires url_env and no sqlite_path")
		}
		o.URL = os.Getenv(db.URLEnv)
		u, e := url.Parse(o.URL)
		if e != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Host == "" || u.Path == "" {
			return o, errors.New("database URL environment variable must contain a valid PostgreSQL URI")
		}
		if !demo && u.Query().Get("sslmode") != "verify-full" {
			return o, errors.New("production PostgreSQL URI requires sslmode=verify-full")
		}
	default:
		return o, errors.New("unknown database driver")
	}
	return o, nil
}
func validateBroker(c core.Config, demo bool) ([]core.Config, error) {
	configs, e := c.WorkspaceConfigs(demo)
	if e != nil {
		return nil, e
	}
	host, _, e := net.SplitHostPort(c.Listen)
	if e != nil {
		return nil, errors.New("listen must be host:port")
	}
	if demo {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return nil, errors.New("demo must listen on a loopback IP")
		}
	} else {
		if c.StateFile != "" || c.DemoFile != "" {
			return nil, errors.New("production forbids demo state_file and demo_file")
		}
		if _, e = tls.LoadX509KeyPair(c.TLSCert, c.TLSKey); e != nil {
			return nil, errors.New("valid Broker TLS certificate and key required")
		}
		if e = privateFile(c.TLSKey); e != nil {
			return nil, e
		}
		pem, e := os.ReadFile(c.ClientCA)
		if e != nil {
			return nil, e
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("invalid Agent client CA")
		}
	}
	if _, e = databaseOptions(c, demo, "default"); e != nil {
		return nil, e
	}
	roots := map[string]bool{}
	for _, cfg := range configs {
		if !demo && (!core.ValidWorkspaceID(cfg.WorkspaceID) || len(cfg.Projects) == 0) {
			return nil, errors.New("production requires an explicit workspace_id or workspaces[].id")
		}
		key, e := core.ReadKey(cfg.PrivateKeyFile)
		if e != nil {
			return nil, e
		}
		if !demo {
			if e = privateFile(cfg.PrivateKeyFile); e != nil {
				return nil, e
			}
		}
		id := core.Hash(key.Public().(ed25519.PublicKey))
		if roots[id] {
			return nil, errors.New("workspaces require distinct Broker signing keys")
		}
		roots[id] = true
	}
	for _, cfg := range configs {
		for _, p := range cfg.Principals {
			if roots[core.PublicKeyID(p.PublicKey)] {
				return nil, errors.New("Broker keys cannot be used as approval keys in any workspace")
			}
		}
	}
	return configs, nil
}
