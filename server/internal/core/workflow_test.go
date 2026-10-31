package core

import (
	"encoding/hex"
	"strings"
	"testing"
	"testing/quick"
	"unicode/utf8"
)

func diagnosticFixture() DiagnosticPolicy {
	return DiagnosticPolicy{Roots: []DiagnosticRoot{{ID: "logs", Path: "/srv/app/logs"}}, Tools: []string{"ls", "cat", "tail", "grep", "status"}, AutoTools: []string{"ls", "status"}}
}
func TestTerminalParsing(t *testing.T) {
	p := diagnosticFixture()
	for line, want := range map[string]DiagnosticOperation{"ls": {Tool: "ls", Resource: "logs", Path: "."}, "tail -n 20 logs:app.log": {Tool: "tail", Resource: "logs", Path: "app.log", Lines: 20}, "grep 'error message' /srv/app/logs/app.log": {Tool: "grep", Resource: "logs", Path: "app.log", Query: "error message"}, "status": {Tool: "status"}} {
		got, e := p.Parse(line)
		if e != nil || string(JSON(got)) != string(JSON(want)) {
			t.Fatalf("%s: %+v %v", line, got, e)
		}
	}
	for _, line := range []string{"cat /etc/shadow", "cat logs:../secret", "cat logs:a/../../secret", "cat logs:a//b", "ls logs:./foo", "ls /srv/app/logs-other", "ls;id", "ls | sh", "cat $(id)", "cat `id`", "cat logs:foo > x", "grep '[human]' logs:app.log &", "tail -n 201 logs:app.log", "cat logs:.", "ls logs:foo\\bar", "grep 'unclosed logs:app.log", "status extra", "sh -c id"} {
		if _, e := p.Parse(line); e == nil {
			t.Fatalf("accepted %q", line)
		}
	}
}
func TestPathConfinementProperties(t *testing.T) {
	p := diagnosticFixture()
	// Every generated safe filename parses faithfully, while every traversal
	// prefix is refused. No rejection filtering hides the valid-path cases.
	if e := quick.Check(func(data []byte, n uint8) bool {
		name := "file-" + hex.EncodeToString(data)
		op, e := p.Parse("cat logs:" + name)
		if e != nil || op.Path != name {
			return false
		}
		_, e = p.Parse("cat logs:" + strings.Repeat("../", int(n%20)+1) + name)
		return e != nil
	}, &quick.Config{MaxCount: 1000}); e != nil {
		t.Fatal(e)
	}
}
func TestRiskContextScopeRedactionAndBudget(t *testing.T) {
	i := Intent{ID: "new", Requester: "alice", WorkspaceID: "w", ProjectID: "p", Target: "s"}
	st := &State{}
	for n := 0; n < 15; n++ {
		old := i
		old.ID = ID()
		st.Requests = append(st.Requests, Request{Intent: old, Executions: []Execution{{StartedAt: 100000 - int64(n), Output: "password=supersecret\nAuthorization: Bearer hidden-value\n-----BEGIN PRIVATE KEY-----\nsecret\n-----END PRIVATE KEY-----\n" + strings.Repeat("日志", 1000)}}})
	}
	for _, field := range []string{"requester", "workspace", "project", "server", "old"} {
		old := i
		old.ID = ID()
		at := int64(100001)
		switch field {
		case "requester":
			old.Requester = "mallory"
		case "workspace":
			old.WorkspaceID = "other"
		case "project":
			old.ProjectID = "other"
		case "server":
			old.Target = "other"
		case "old":
			at = 1
		}
		st.Requests = append(st.Requests, Request{Intent: old, Executions: []Execution{{StartedAt: at, Output: "CROSS_SCOPE_SECRET"}}})
	}
	c := CollectRiskContext(st, i, 100002)
	if len(c.Records) != 10 || !c.Truncated {
		t.Fatal("not bounded")
	}
	total := 0
	for _, r := range c.Records {
		total += len(r.Output)
		if !utf8.ValidString(r.Output) {
			t.Fatal("invalid UTF8")
		}
	}
	raw := string(JSON(c))
	for _, secret := range []string{"supersecret", "hidden-value", "PRIVATE KEY", "CROSS_SCOPE_SECRET"} {
		if strings.Contains(raw, secret) {
			t.Fatal("leaked " + secret)
		}
	}
	if total > 8192 || c.Digest() == "" {
		t.Fatal("budget or digest")
	}
	before := c.Digest()
	st.Requests[0].Executions[0].Output = "changed"
	if before != c.Digest() {
		t.Fatal("snapshot mutated")
	}
}
func TestNewIntentRoundtrip(t *testing.T) {
	if e := quick.Check(func(data []byte) bool {
		p := diagnosticFixture()
		i := Intent{Version: 4, Kind: "command", Scope: &p, Operation: &DiagnosticOperation{Tool: "grep", Resource: "logs", Path: "app.log", Query: hex.EncodeToString(data)}}
		got, e := DecodeIntent(Encode(i))
		return e == nil && string(JSON(got)) == string(JSON(i))
	}, nil); e != nil {
		t.Fatal(e)
	}
}
