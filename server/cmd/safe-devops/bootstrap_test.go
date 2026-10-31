package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"os"
	"path/filepath"
	"safeops/internal/agent"
	"safeops/internal/core"
	"safeops/internal/device"
	"strings"
	"testing"
)

func TestComposeBootstrapAndProductionPreflight(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	approval := base64.StdEncoding.EncodeToString(pub)
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "production")
			if e := initCompose(dir, "acme", "payments", "https://broker.example.com:8443", "https://ops.example.com", approval, driver); e != nil {
				t.Fatal(e)
			}
			original, e := os.ReadFile(filepath.Join(dir, "broker/acme-broker.key"))
			if e != nil {
				t.Fatal(e)
			}
			if initCompose(dir, "acme", "payments", "https://broker.example.com:8443", "https://ops.example.com", approval, driver) == nil {
				t.Fatal("overwrote existing secrets")
			}
			after, _ := os.ReadFile(filepath.Join(dir, "broker/acme-broker.key"))
			if string(original) != string(after) {
				t.Fatal("secrets changed")
			}
			if e := filepath.Walk(dir, func(path string, info os.FileInfo, e error) error {
				if e != nil {
					return e
				}
				if info.Mode().Perm()&0077 != 0 {
					t.Errorf("unprotected bootstrap file: %s", path)
				}
				return nil
			}); e != nil {
				t.Fatal(e)
			}
			if _, e := os.Stat(filepath.Join(dir, "credentials/reviewer.key")); !os.IsNotExist(e) {
				t.Fatal("human private key generated")
			}
			var c core.Config
			if e := load(filepath.Join(dir, "broker/broker.json"), &c); e != nil {
				t.Fatal(e)
			}
			c.TLSCert = filepath.Join(dir, "broker/broker.crt")
			c.TLSKey = filepath.Join(dir, "broker/broker-tls.key")
			c.ClientCA = filepath.Join(dir, "broker/connector-ca.crt")
			c.Workspaces[0].PrivateKeyFile = filepath.Join(dir, "broker/acme-broker.key")
			t.Setenv("SAFEOPS_DATABASE_URL", "")
			if driver == "postgres" {
				env, _ := os.ReadFile(filepath.Join(dir, "broker/broker.env"))
				t.Setenv("SAFEOPS_DATABASE_URL", strings.TrimSpace(strings.TrimPrefix(string(env), "SAFEOPS_DATABASE_URL=")))
			}
			if _, e := validateBroker(c, false); e != nil {
				t.Fatal(e)
			}
			var a core.AgentConfig
			if e := load(filepath.Join(dir, "connector/connector.json"), &a); e != nil {
				t.Fatal(e)
			}
			a.TLSCert = filepath.Join(dir, "connector/connector.crt")
			a.TLSKey = filepath.Join(dir, "connector/connector-tls.key")
			a.CA = filepath.Join(dir, "connector/broker-ca.crt")
			if e := agent.ValidateConfig(a, false); e != nil {
				t.Fatal(e)
			}
			for _, item := range []struct {
				path, ca, host string
				usage          x509.ExtKeyUsage
			}{{"broker/broker.crt", "offline-ca/server-ca.crt", "broker", x509.ExtKeyUsageServerAuth}, {"gateway/web.crt", "offline-ca/server-ca.crt", "ops.example.com", x509.ExtKeyUsageServerAuth}, {"postgres/server.crt", "offline-ca/server-ca.crt", "postgres", x509.ExtKeyUsageServerAuth}, {"connector/connector.crt", "offline-ca/connector-ca.crt", "", x509.ExtKeyUsageClientAuth}} {
				b, _ := os.ReadFile(filepath.Join(dir, item.path))
				block, _ := pem.Decode(b)
				cert, e := x509.ParseCertificate(block.Bytes)
				if e != nil {
					t.Fatal(e)
				}
				b, _ = os.ReadFile(filepath.Join(dir, item.ca))
				roots := x509.NewCertPool()
				roots.AppendCertsFromPEM(b)
				if _, e = cert.Verify(x509.VerifyOptions{Roots: roots, DNSName: item.host, KeyUsages: []x509.ExtKeyUsage{item.usage}}); e != nil {
					t.Fatal(e)
				}
				if item.usage == x509.ExtKeyUsageClientAuth && cert.Subject.CommonName != a.ID {
					t.Fatal("connector certificate does not match configured identity")
				}
			}
			if driver == "sqlite" {
				c.Database.URLEnv = "SAFEOPS_DATABASE_URL"
				if _, e := databaseOptions(c, false, "acme"); e == nil {
					t.Fatal("mixed database configuration accepted")
				}
			} else {
				t.Setenv("SAFEOPS_DATABASE_URL", "postgres://user:pw@db/safeops?sslmode=disable")
				if _, e := databaseOptions(c, false, "acme"); e == nil {
					t.Fatal("insecure PostgreSQL TLS accepted")
				}
			}
		})
	}
}
func TestBootstrapRejectsInvalidInputsWithoutCreatingDirectory(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	approval := base64.StdEncoding.EncodeToString(pub)
	for _, input := range []struct{ workspace, origin, web, driver string }{{"../bad", "https://b:8443", "https://w", "sqlite"}, {"acme", "http://b:8443", "https://w", "sqlite"}, {"acme", "https://:8443", "https://w", "sqlite"}, {"acme", "https://b:8443", "https://w", "mysql"}, {"acme", "https://b", "https://w", "sqlite"}, {"acme", "https://b:8443", "https://w/path", "sqlite"}} {
		dir := filepath.Join(t.TempDir(), "new")
		if initCompose(dir, input.workspace, "core", input.origin, input.web, approval, input.driver) == nil {
			t.Fatal("invalid input accepted")
		}
		if _, e := os.Stat(dir); !os.IsNotExist(e) {
			t.Fatal("invalid input wrote bootstrap files")
		}
	}
}

func TestBootstrapEnrolsSeparateDeviceAndReviewerKeys(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	_, machine, _ := ed25519.GenerateKey(rand.Reader)
	dir := filepath.Join(t.TempDir(), "devices")
	if e := initConfig(dir, "team", "project", "https://broker.example.com", base64.StdEncoding.EncodeToString(pub), "sqlite", "/srv/app/logs", device.Public(machine)); e != nil {
		t.Fatal(e)
	}
	var cfg core.Config
	if e := load(filepath.Join(dir, "broker.json"), &cfg); e != nil {
		t.Fatal(e)
	}
	policy := cfg.Workspaces[0].DevicePolicy
	if policy == nil || !policy.Required || len(policy.Devices) != 2 {
		t.Fatal("device enrollment missing")
	}
	if policy.Devices[0].ID != core.DeviceID(machine.Public().(ed25519.PublicKey)) || policy.Devices[1].Principals[0] != "reviewer" {
		t.Fatal("device identity binding wrong")
	}
	if _, e := cfg.WorkspaceConfigs(false); e != nil {
		t.Fatal(e)
	}
}
