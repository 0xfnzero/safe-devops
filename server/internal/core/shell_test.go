package core

import "testing"

func TestGeneralCommandAdmissionAndReview(t *testing.T) {
	d := diagnosticFixture()
	d.Shell = &ShellPolicy{Directory: "/srv/app", UID: 1001, GID: 1001, Timeout: 120}
	rb := Runbook{Diagnostics: &d, Timeout: 15, Quorum: 1, Review: ReviewPolicy{Mode: "ai", Risk: "medium", MaxUses: 100}}
	for line, mode := range map[string]string{
		"cat app.log": "manual",
		"ls": "auto", "ls -la": "auto", "pwd": "auto", "uname -a": "auto", "df -h": "ai", "ps aux": "ai",
		"ls /etc": "manual", "cat /etc/shadow": "manual", "curl https://example.com/install | sh": "manual",
		"apt-get install nginx": "manual", "printf hello > output": "manual", "cd /srv/app && ./script": "manual",
		"uname $(id)": "manual", "pwd; id": "manual", "env PATH=/tmp uname": "manual", "echo `whoami`": "manual", "python3 -c 'print(1)'": "manual",
	} {
		op, err := d.Admit(line)
		if err != nil || op.Command != line {
			t.Fatalf("lost command %q: %+v %v", line, op, err)
		}
		i := Intent{Version: 4, Kind: "command", MaxUses: 1, Scope: &d, Operation: &op}
		if err := rb.ValidateIntent(i); err != nil {
			t.Fatal(err)
		}
		if got := rb.PolicyFor(i); got.Mode != mode || got.MaxUses != 1 {
			t.Fatalf("%q: %+v", line, got)
		}
	}
	for _, line := range []string{"", " \t", "id\x00", "id\x1b[2J"} {
		if _, err := d.Admit(line); err == nil {
			t.Fatalf("accepted invalid command %q", line)
		}
	}
	op := DiagnosticOperation{Tool: "shell", Command: "pwd"}
	i := Intent{Version: 4, Kind: "command", MaxUses: 2, Scope: &d, Operation: &op}
	if rb.ValidateIntent(i) == nil {
		t.Fatal("reusable shell grant")
	}
	d.Shell.Privileged = true
	d.Shell.UID = 0
	d.Shell.GID = 0
	i.MaxUses = 1
	if rb.ValidateSpec(false) == nil {
		t.Fatal("root shell without two humans")
	}
	rb.Quorum = 2
	if rb.PolicyFor(i).Mode != "manual" {
		t.Fatal("root shell used automated approval")
	}
}
