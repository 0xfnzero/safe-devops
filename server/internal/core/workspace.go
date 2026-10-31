package core

import (
	"errors"
	"regexp"
)

type DatabaseConfig struct {
	Driver     string `json:"driver"`
	SQLitePath string `json:"sqlite_path,omitempty"`
	URLEnv     string `json:"url_env,omitempty"`
}
type Project struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Members     []string `json:"members"`
}
type Workspace struct {
	DevicePolicy    *DevicePolicy   `json:"device_policy,omitempty"`
	RiskLockSeconds int             `json:"risk_lock_seconds,omitempty"`
	Projects        []Project       `json:"projects"`
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	PrivateKeyFile  string          `json:"private_key_file"`
	Principals      []Principal     `json:"principals"`
	Agents          []AgentIdentity `json:"agents"`
	Targets         []Target        `json:"targets"`
}

var workspaceIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

func ValidWorkspaceID(id string) bool { return workspaceIDPattern.MatchString(id) }

// Each workspace gets its own Broker policy, signing root and storage partition.
// Routing uses authenticated identity; caller-supplied workspace headers are ignored.
func (c Config) WorkspaceConfigs(demo bool) ([]Config, error) {
	if len(c.Workspaces) == 0 {
		if c.WorkspaceID != "" && !ValidWorkspaceID(c.WorkspaceID) {
			return nil, errors.New("invalid workspace_id")
		}
		if e := c.Validate(demo); e != nil {
			return nil, e
		}
		return []Config{c}, nil
	}
	if len(c.Principals) > 0 || len(c.Agents) > 0 || len(c.Targets) > 0 || c.PrivateKeyFile != "" || c.WorkspaceID != "" || c.StateFile != "" {
		return nil, errors.New("workspaces cannot be mixed with legacy single-workspace fields")
	}
	seen, tokens, agents, keys := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	out := []Config{}
	for _, w := range c.Workspaces {
		if !ValidWorkspaceID(w.ID) || seen[w.ID] || w.Name == "" || len(w.Name) > 100 || w.PrivateKeyFile == "" {
			return nil, errors.New("invalid or duplicate workspace")
		}
		seen[w.ID] = true
		cfg := Config{DevicePolicy: w.DevicePolicy, RiskLockSeconds: w.RiskLockSeconds, Projects: w.Projects, WorkspaceID: w.ID, WorkspaceName: w.Name, Principals: w.Principals, Agents: w.Agents, Targets: w.Targets, PrivateKeyFile: w.PrivateKeyFile}
		if e := cfg.Validate(demo); e != nil {
			return nil, e
		}
		for _, p := range w.Principals {
			if tokens[p.TokenHash] {
				return nil, errors.New("identity tokens must be unique across workspaces")
			}
			tokens[p.TokenHash] = true
			if p.PublicKey != "" {
				id := PublicKeyID(p.PublicKey)
				if keys[id] {
					return nil, errors.New("approval keys must be isolated between workspaces")
				}
				keys[id] = true
			}
		}
		for _, a := range w.Agents {
			if agents[a.ID] {
				return nil, errors.New("agent certificate identities must be unique across workspaces")
			}
			agents[a.ID] = true
			if demo {
				if tokens[a.TokenHash] {
					return nil, errors.New("agent token reused across workspaces")
				}
				tokens[a.TokenHash] = true
			}
		}
		out = append(out, cfg)
	}
	return out, nil
}
