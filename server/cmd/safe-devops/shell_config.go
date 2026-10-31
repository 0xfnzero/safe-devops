package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"safeops/internal/core"
)

// Stage a coordinated policy update without overwriting live configuration or
// modifying its database. Review/install the generated files on each host.
func configureShell(brokerPath, connectorPath, output, profile string, shell core.ShellPolicy) error {
	if err := shell.Validate(); err != nil {
		return err
	}
	if shell.Demo || !core.ValidWorkspaceID(profile) {
		return errors.New("production shell policy and valid profile ID required")
	}
	var brokerConfig core.Config
	var connector core.AgentConfig
	if err := load(brokerPath, &brokerConfig); err != nil {
		return err
	}
	if err := load(connectorPath, &connector); err != nil {
		return err
	}
	targets := &brokerConfig.Targets
	if len(brokerConfig.Workspaces) > 0 {
		targets = nil
		for n := range brokerConfig.Workspaces {
			if brokerConfig.Workspaces[n].ID == connector.WorkspaceID {
				targets = &brokerConfig.Workspaces[n].Targets
				break
			}
		}
	}
	if targets == nil {
		return errors.New("Connector workspace does not exist in Broker config")
	}
	var target *core.Target
	for n := range *targets {
		if (*targets)[n].ID == connector.Target && (*targets)[n].ProjectID == connector.ProjectID {
			target = &(*targets)[n]
			break
		}
	}
	if target == nil {
		return errors.New("Connector server/project does not exist in Broker config")
	}
	rb, ok := connector.Runbooks[profile]
	if !ok || rb.Diagnostics == nil {
		return errors.New("profile must name an existing diagnostic runbook; initialize with --diagnostic-root first")
	}
	rb.Diagnostics.Shell = &shell
	if err := rb.ValidateSpec(false); err != nil {
		return err
	}
	found := false
	for n := range target.Actions {
		if target.Actions[n].ID == profile {
			a := &target.Actions[n]
			a.Diagnostics = rb.Diagnostics
			a.CommandHash = rb.Hash()
			a.Description = "Built-in diagnostics plus arbitrary shell commands with exact, single-use review"
			found = true
		}
	}
	if !found {
		return errors.New("diagnostic profile missing from Broker catalog")
	}
	connector.Runbooks[profile] = rb
	connector.ChangeSocket = "/run/safe-devops-change/runner.sock"
	runner := connector
	runner.Runbooks = map[string]core.Runbook{profile: rb}
	runner.LedgerFile = "/var/lib/safe-devops-change-runner/ledger.json"
	if _, err := brokerConfig.WorkspaceConfigs(false); err != nil {
		return err
	}
	output, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	if err = os.Mkdir(output, 0700); err != nil {
		return errors.New("output directory must be new; existing configuration is never overwritten")
	}
	for name, value := range map[string]any{"broker.json": brokerConfig, "connector.json": connector, "change-runner.json": runner} {
		if err = writeJSON(filepath.Join(output, name), value); err != nil {
			return err
		}
	}
	fmt.Printf("Staged reviewed-shell configuration: %s\nPolicy hash: %s\nInstall on the corresponding hosts and apply the Broker policy migration. See docs/terminal.md.\n", output, rb.Hash())
	return nil
}
