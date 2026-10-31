package broker

import (
	"testing"
	"time"

	"safeops/internal/agent"
	"safeops/internal/core"
)

func TestShellApprovalCannotBeBypassedOrReplaced(t *testing.T) {
	f := workflowFixture(t, &core.ShellPolicy{Directory: "/srv/app", UID: 1001, GID: 1001, Timeout: 120})
	r := f.command("printf hello | cat > result.txt")
	if r.Status != "pending" || r.Review.Mode != "manual" || !r.NeedsHuman || r.Intent.Operation.Command != "printf hello | cat > result.txt" {
		t.Fatalf("unexpected admission: %+v", r)
	}
	if f.poll("agent") != nil {
		t.Fatal("unapproved shell queued")
	}
	f.call("machine", "POST", "/api/requests/"+r.Intent.ID+"/review", core.ReviewSubmission{Decision: "approve", Reason: "safe", Model: "test", Signature: core.Sign(f.keys["machine"], core.ApprovalDomain, r.Payload)}, 403, nil)
	f.approve(r, "bob", 200)
	env := f.poll("agent")
	if env == nil {
		t.Fatal("approved shell not queued")
	}
	if _, _, err := agent.Verify(f.config, *env, time.Now()); err != nil {
		t.Fatal(err)
	}
	if env.Lease.ExpiresAt < time.Now().Unix()+115 {
		t.Fatal("shell execution timeout was replaced by diagnostic timeout")
	}
	for _, attack := range []string{"text", "directory", "uid", "lease-operation", "ai-signature"} {
		tampered := *env
		i, _ := core.DecodeIntent(env.Lease.Payload)
		switch attack {
		case "text":
			i.Operation.Command = "cat /etc/shadow"
		case "directory":
			i.Scope.Shell.Directory = "/etc"
		case "uid":
			i.Scope.Shell.UID = 0
		case "lease-operation":
			op := *env.Lease.Operation
			op.Command = "id"
			tampered.Lease.Operation = &op
		case "ai-signature":
			tampered.Lease.Approvals = []core.Approval{{Subject: "machine", Kind: "human", Signature: core.Sign(f.keys["machine"], core.ApprovalDomain, r.Payload)}}
		}
		tampered.Lease.Payload = core.Encode(i)
		tampered.Signature = core.Sign(f.keys["broker"], core.LeaseDomain, core.Encode(tampered.Lease))
		if _, _, err := agent.Verify(f.config, tampered, time.Now()); err == nil {
			t.Fatal("accepted " + attack)
		}
	}
	f.result(r, env, 0)
}

func TestSessionDoesNotAuthorizeGeneralCommands(t *testing.T) {
	f := workflowFixture(t, &core.ShellPolicy{Directory: "/srv/app", UID: 1001, GID: 1001, Timeout: 120})
	var session core.Request
	f.call("alice", "POST", "/api/requests", map[string]any{"target": "t1", "action": "diagnostics", "reason": "Investigate app failures", "max_uses": 3}, 200, &session)
	f.approve(session, "bob", 200)
	f.call("alice", "POST", "/api/requests/"+session.Intent.ID+"/command", core.DiagnosticOperation{Tool: "shell", Command: "pwd"}, 400, nil)
	var separate core.Request
	f.call("alice", "POST", "/api/terminal/command", map[string]any{"target": "t1", "profile": "diagnostics", "command": "touch change.txt", "session": session.Intent.ID}, 200, &separate)
	if separate.Intent.ID == session.Intent.ID || separate.Status != "pending" || !separate.NeedsHuman {
		t.Fatal("session authorized a shell change")
	}
}
