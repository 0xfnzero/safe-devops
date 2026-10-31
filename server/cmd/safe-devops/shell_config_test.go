package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"safeops/internal/core"
)

func TestConfigureShellStagesMatchingPoliciesWithoutOverwriting(t *testing.T) {
	public, _, _ := ed25519.GenerateKey(rand.Reader)
	root := t.TempDir()
	dir := filepath.Join(root, "original")
	if err := initConfig(dir, "acme", "core", "https://broker.example.com", base64.StdEncoding.EncodeToString(public), "sqlite", "/srv/app/logs"); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "connector.json")
	original, _ := os.ReadFile(source)
	output := filepath.Join(root, "staged")
	p := core.ShellPolicy{Directory: "/srv/app", UID: 1001, GID: 1001, Timeout: 120}
	if err := configureShell(filepath.Join(dir, "broker.json"), source, output, "diagnostics", p); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(source)
	if string(original) != string(after) {
		t.Fatal("modified active config")
	}
	var b core.Config
	var a, r core.AgentConfig
	if err := load(filepath.Join(output, "broker.json"), &b); err != nil {
		t.Fatal(err)
	}
	if err := load(filepath.Join(output, "connector.json"), &a); err != nil {
		t.Fatal(err)
	}
	if err := load(filepath.Join(output, "change-runner.json"), &r); err != nil {
		t.Fatal(err)
	}
	if len(r.Runbooks) != 1 || r.LedgerFile == a.LedgerFile || r.ChangeSocket != a.ChangeSocket {
		t.Fatal("runner isolation configuration")
	}
	hash := a.Runbooks["diagnostics"].Hash()
	if hash != r.Runbooks["diagnostics"].Hash() || hash != b.Workspaces[0].Targets[0].Actions[1].CommandHash {
		t.Fatal("policy mismatch")
	}
	if configureShell(filepath.Join(dir, "broker.json"), source, output, "diagnostics", p) == nil {
		t.Fatal("overwrote staged config")
	}
	p.Privileged = true
	p.UID = 0
	p.GID = 0
	if configureShell(filepath.Join(dir, "broker.json"), source, filepath.Join(root, "unsafe"), "diagnostics", p) == nil {
		t.Fatal("enabled root without two humans")
	}
}
