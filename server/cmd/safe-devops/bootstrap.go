package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"golang.org/x/crypto/ssh"
	"net/url"
	"os"
	"path/filepath"
	"safeops/internal/core"
	"strings"
)

// The human private key must already exist on an independent approval device.
// Bootstrap accepts only its public key and never creates a shared human key.
func initConfig(dir, workspace, project, origin, approvalPublic, database string, diagnosticRoot ...string) error {
	if !core.ValidWorkspaceID(workspace) || !core.ValidWorkspaceID(project) || core.PublicKeyID(approvalPublic) == "" {
		return errors.New("valid --workspace, --project and independent --approver-public-key required")
	}
	var diagnostic *core.DiagnosticPolicy
	if len(diagnosticRoot) > 0 && diagnosticRoot[0] != "" {
		diagnostic = &core.DiagnosticPolicy{Roots: []core.DiagnosticRoot{{ID: "logs", Path: diagnosticRoot[0]}}, Tools: []string{"ls", "cat", "tail", "grep", "status"}, AutoTools: []string{"ls", "status"}}
		if err := diagnostic.Validate(); err != nil {
			return err
		}
	}
	var devicePolicy *core.DevicePolicy
	if len(diagnosticRoot) > 1 && diagnosticRoot[1] != "" {
		pub, e := core.DevicePublic(diagnosticRoot[1])
		if e != nil {
			return e
		}
		reviewRaw, e := base64.StdEncoding.DecodeString(approvalPublic)
		if e != nil {
			return e
		}
		reviewPub := ed25519.PublicKey(reviewRaw)
		sshKey, e := ssh.NewPublicKey(reviewPub)
		if e != nil {
			return e
		}
		devicePolicy = &core.DevicePolicy{Required: true, Devices: []core.Device{{ID: core.DeviceID(pub), Name: "Development / operations workstation", PublicKey: diagnosticRoot[1], Principals: []string{"developer", "auditor"}}}}
		if core.DeviceID(pub) == core.DeviceID(reviewPub) {
			return errors.New("requester machine key must differ from independent approval key")
		}
		devicePolicy.Devices = append(devicePolicy.Devices, core.Device{ID: core.DeviceID(reviewPub), Name: "Independent approval device", PublicKey: strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshKey))), Principals: []string{"reviewer"}})
	}
	u, e := url.Parse(origin)
	if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return errors.New("--broker-url must be an HTTPS origin")
	}
	if database != "sqlite" && database != "postgres" {
		return errors.New("--database must be sqlite or postgres")
	}
	dir, e = filepath.Abs(dir)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(dir), 0700); e != nil {
		return e
	}
	if e = os.Mkdir(dir, 0700); e != nil {
		return errors.New("configuration destination must be new; existing secrets are never overwritten")
	}
	public, e := keyFile(filepath.Join(dir, workspace+"-broker.key"))
	if e != nil {
		return e
	}
	principals := []core.Principal{}
	for _, identity := range []struct{ id, role string }{{"developer", "requester"}, {"reviewer", "approver"}, {"auditor", "auditor"}} {
		token := core.ID() + core.ID()
		p := core.Principal{ID: identity.id, Name: identity.id, Roles: []string{identity.role}, TokenHash: core.Hash([]byte(token)), Kind: "human"}
		if identity.role == "requester" {
			p.Name = "Developer / Operator"
		}
		if identity.role == "approver" {
			p.PublicKey = approvalPublic
		}
		if e = os.WriteFile(filepath.Join(dir, identity.id+".token"), []byte(token+"\n"), 0600); e != nil {
			return e
		}
		principals = append(principals, p)
	}
	rb := core.Runbook{Argv: []string{"/usr/bin/uname", "-srm"}, Timeout: 10, Requesters: []string{"developer"}, Approvers: []string{"reviewer"}, Quorum: 1, Review: core.ReviewPolicy{Mode: "manual", Risk: "high", MaxUses: 1, RateLimit: 6}}
	w := core.Workspace{DevicePolicy: devicePolicy, ID: workspace, Name: workspace, PrivateKeyFile: "/etc/safe-devops/" + workspace + "-broker.key", Principals: principals, Projects: []core.Project{{ID: project, Name: project, Members: []string{"developer", "reviewer"}}}, Agents: []core.AgentIdentity{{ID: workspace + "-connector-01", Target: "server-01"}}, Targets: []core.Target{{ID: "server-01", ProjectID: project, Name: "Server 01", Environment: "PRODUCTION", Requesters: rb.Requesters, Actions: []core.Action{{ID: "system-info", Name: "System information", Description: "Fixed diagnostic; independent human approval required", CommandHash: core.CommandHash(rb.Argv, rb.Timeout), Timeout: rb.Timeout, Approvers: rb.Approvers, Quorum: rb.Quorum, Review: rb.Review}}}}}
	db := core.DatabaseConfig{Driver: database, SQLitePath: "/var/lib/safe-devops-broker/safeops.db"}
	env := ""
	if database == "postgres" {
		db.SQLitePath = ""
		db.URLEnv = "SAFEOPS_DATABASE_URL"
		env = "SAFEOPS_DATABASE_URL=postgres://REPLACE@db.internal/safeops?sslmode=verify-full&sslrootcert=/etc/safe-devops/db-ca.crt\n"
	}
	c := core.Config{Listen: "0.0.0.0:8443", Database: db, Workspaces: []core.Workspace{w}, TLSCert: "/etc/safe-devops/broker.crt", TLSKey: "/etc/safe-devops/broker-tls.key", ClientCA: "/etc/safe-devops/connector-ca.crt"}
	a := core.AgentConfig{WorkspaceID: workspace, ProjectID: project, ID: workspace + "-connector-01", Target: "server-01", BrokerURL: origin, BrokerKey: public, ApproverKeys: map[string]string{"reviewer": approvalPublic}, ApproverKinds: map[string]string{"reviewer": "human"}, Runbooks: map[string]core.Runbook{"system-info": rb}, LedgerFile: "/var/lib/safe-devops-connector/ledger.json", TLSCert: "/etc/safe-devops/connector.crt", TLSKey: "/etc/safe-devops/connector-tls.key", CA: "/etc/safe-devops/broker-ca.crt"}
	if diagnostic != nil {
		profile := core.Runbook{Diagnostics: diagnostic, Timeout: 15, Requesters: []string{"developer"}, Approvers: []string{"reviewer"}, Quorum: 1, Review: core.ReviewPolicy{Mode: "manual", Risk: "medium", MaxUses: 100, RateLimit: 30}}
		a.Runbooks["diagnostics"] = profile
		c.Workspaces[0].Targets[0].Actions = append(c.Workspaces[0].Targets[0].Actions, core.Action{ID: "diagnostics", Name: "Diagnostic terminal", Description: "Scoped diagnostics; ls/status pass policy, other commands require independent review", Diagnostics: diagnostic, CommandHash: profile.Hash(), Timeout: profile.Timeout, Approvers: profile.Approvers, Quorum: profile.Quorum, Review: profile.Review})
	}
	if e = writeJSON(filepath.Join(dir, "broker.json"), c); e != nil {
		return e
	}
	if e = writeJSON(filepath.Join(dir, "connector.json"), a); e != nil {
		return e
	}
	if e = os.WriteFile(filepath.Join(dir, "broker.env"), []byte(env), 0600); e != nil {
		return e
	}
	if e = os.WriteFile(filepath.Join(dir, "web.env"), []byte("SAFEOPS_API_ORIGIN="+origin+"\nNODE_EXTRA_CA_CERTS=/etc/safe-devops/broker-ca.crt\n"), 0600); e != nil {
		return e
	}
	fmt.Printf("Configuration created at %s\nInstall TLS certificates and each component's private files on its own host; run check-config before starting. Human approval private key was not copied.\n", dir)
	return nil
}
