package broker

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"safeops/internal/agent"
	"safeops/internal/core"
	"safeops/internal/store"
	"testing"
	"time"
)

func TestWorkspaceProjectAndServerIsolation(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "workspaces.db")
	brokers := []*Broker{}
	configs := map[string]core.AgentConfig{}
	for _, workspace := range []string{"alpha", "beta"} {
		pub, key, _ := ed25519.GenerateKey(rand.Reader)
		c := core.Config{WorkspaceID: workspace, WorkspaceName: workspace, Projects: []core.Project{{ID: "payments", Name: "Payments", Members: []string{"alice"}}, {ID: "reports", Name: "Reports", Members: []string{"dave"}}}, Principals: []core.Principal{{ID: "alice", Roles: []string{"requester"}, TokenHash: core.Hash([]byte(workspace + "-alice"))}, {ID: "dave", Roles: []string{"requester"}, TokenHash: core.Hash([]byte(workspace + "-dave"))}, {ID: "audit", Roles: []string{"auditor"}, TokenHash: core.Hash([]byte(workspace + "-audit"))}}}
		rb := core.Runbook{Argv: []string{"/usr/bin/uname", "-s"}, Timeout: 5, Requesters: []string{"alice"}, Review: core.ReviewPolicy{Mode: "auto", Risk: "low", MaxUses: 1, RateLimit: 6}}
		action := core.Action{ID: "status", CommandHash: core.CommandHash(rb.Argv, rb.Timeout), Timeout: 5, Review: rb.Review}
		for n, id := range []string{"web-1", "web-2", "reports-1"} {
			project, member := "payments", "alice"
			if n == 2 {
				project, member = "reports", "dave"
			}
			c.Targets = append(c.Targets, core.Target{ID: id, ProjectID: project, Requesters: []string{member}, Actions: []core.Action{action}})
			c.Agents = append(c.Agents, core.AgentIdentity{ID: workspace + "-" + id, Target: id, TokenHash: core.Hash([]byte(workspace + "-agent-" + id))})
		}
		st, e := store.OpenDatabase(context.Background(), store.Options{Driver: "sqlite", Path: dbPath, Workspace: workspace})
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { st.Close() })
		b, e := New(context.Background(), c, st, key, true)
		if e != nil {
			t.Fatal(e)
		}
		brokers = append(brokers, b)
		configs[workspace] = core.AgentConfig{WorkspaceID: workspace, ProjectID: "payments", ID: workspace + "-web-1", Target: "web-1", BrokerKey: base64.StdEncoding.EncodeToString(pub), Runbooks: map[string]core.Runbook{"status": rb}}
	}
	router, e := NewRouter(brokers, true)
	if e != nil {
		t.Fatal(e)
	}
	srv := httptest.NewServer(router)
	defer srv.Close()
	call := func(identity, method, path string, body any, want int, out any) {
		t.Helper()
		var data []byte
		if body != nil {
			data = core.JSON(body)
		}
		req, _ := http.NewRequest(method, srv.URL+path, bytes.NewReader(data))
		req.Header.Set("Authorization", "Bearer "+identity)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Workspace-ID", "beta")
		res, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		if res.StatusCode != want {
			var err any
			json.NewDecoder(res.Body).Decode(&err)
			t.Fatalf("%s %s: %d want %d (%v)", identity, path, res.StatusCode, want, err)
		}
		if out != nil {
			if e = json.NewDecoder(res.Body).Decode(out); e != nil {
				t.Fatal(e)
			}
		}
	}
	var catalog struct {
		Targets []core.Target `json:"targets"`
	}
	call("alpha-alice", "GET", "/api/catalog", nil, 200, &catalog)
	if len(catalog.Targets) != 2 {
		t.Fatal("member catalog leaked another project or omitted same-project server")
	}
	var projects struct {
		Projects []core.Project `json:"projects"`
	}
	call("alpha-alice", "GET", "/api/projects", nil, 200, &projects)
	if len(projects.Projects) != 1 || projects.Projects[0].ID != "payments" {
		t.Fatal("project membership ignored")
	}
	var r core.Request
	body := map[string]any{"target": "web-1", "action": "status"}
	call("alpha-alice", "POST", "/api/requests", body, 200, &r)
	if r.Intent.WorkspaceID != "alpha" || r.Intent.ProjectID != "payments" || r.Intent.Version != 3 {
		t.Fatal("scope not signed or header selected workspace")
	}
	call("beta-alice", "GET", "/api/requests/"+r.Intent.ID, nil, 404, nil)
	call("alpha-dave", "GET", "/api/requests/"+r.Intent.ID, nil, 403, nil)
	call("alpha-dave", "POST", "/api/requests", body, 403, nil)
	call("alpha-alice", "POST", "/api/requests", map[string]any{"target": "reports-1", "action": "status"}, 403, nil)
	call("alpha-alice", "POST", "/api/requests", map[string]any{"target": "web-1", "action": "status", "workspace_id": "beta"}, 400, nil)
	var betaList struct {
		Requests []core.Request `json:"requests"`
	}
	call("beta-audit", "GET", "/api/requests", nil, 200, &betaList)
	if len(betaList.Requests) > 0 {
		t.Fatal("auditor crossed workspace")
	}
	var poll struct {
		Job *core.Envelope `json:"job"`
	}
	call("beta-agent-web-1", "POST", "/api/agent/poll", map[string]any{}, 200, &poll)
	if poll.Job != nil {
		t.Fatal("foreign agent acquired task")
	}
	call("alpha-agent-web-1", "POST", "/api/agent/poll", map[string]any{}, 200, &poll)
	if poll.Job == nil {
		t.Fatal("scoped Agent did not acquire task")
	}
	ac := configs["alpha"]
	if _, _, e = agent.Verify(ac, *poll.Job, time.Now()); e != nil {
		t.Fatal(e)
	}
	ac.WorkspaceID = "beta"
	if _, _, e = agent.Verify(ac, *poll.Job, time.Now()); e == nil {
		t.Fatal("Agent accepted foreign workspace")
	}
	ac = configs["alpha"]
	ac.ProjectID = "reports"
	if _, _, e = agent.Verify(ac, *poll.Job, time.Now()); e == nil {
		t.Fatal("Agent accepted foreign project")
	}
}
