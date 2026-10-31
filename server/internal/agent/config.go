package agent

import (
	"errors"
	"os"
	"path/filepath"
	"safeops/internal/core"
	"syscall"
)

func ValidateConfig(c core.AgentConfig, demo bool) error {
	if c.ID == "" || c.Target == "" || core.PublicKeyID(c.BrokerKey) == "" || !filepath.IsAbs(c.LedgerFile) || len(c.Runbooks) == 0 {
		return errors.New("Connector requires identity, target, Broker key, absolute ledger path and runbooks")
	}
	if !demo {
		if !core.ValidWorkspaceID(c.WorkspaceID) || !core.ValidWorkspaceID(c.ProjectID) || c.DemoToken != "" || c.TLSCert == "" || c.TLSKey == "" {
			return errors.New("production Connector requires workspace, project, mTLS and no demo_token")
		}
		info, e := os.Lstat(c.TLSKey)
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			return errors.New("Connector TLS key requires regular 0600 file")
		}
	}
	if _, e := Client(c.BrokerURL, c.CA, c.TLSCert, c.TLSKey, demo); e != nil {
		return e
	}
	for _, rb := range c.Runbooks {
		if rb.ValidateSpec(false) != nil || len(rb.Requesters) == 0 {
			return errors.New("invalid Connector runbook")
		}
		if rb.Diagnostics != nil && rb.Diagnostics.Shell != nil {
			if !demo && !rb.Diagnostics.Shell.Privileged {
				info, err := os.Stat(c.TLSKey)
				if err != nil {
					return err
				}
				stat, ok := info.Sys().(*syscall.Stat_t)
				if !ok || stat.Uid == rb.Diagnostics.Shell.UID {
					return errors.New("shell execution account must not own Connector credentials")
				}
			}
			if rb.Diagnostics.Shell.Demo && !demo {
				return errors.New("demo shell is forbidden in production")
			}
			if !rb.Diagnostics.Shell.Demo && !filepath.IsAbs(c.ChangeSocket) {
				return errors.New("general commands require a separate execution service and absolute change_socket")
			}
		}
		if rb.Plan != nil && rb.Plan.Privileged && !filepath.IsAbs(c.ChangeSocket) {
			return errors.New("privileged plans require an absolute change_socket")
		}
		if e := rb.Review.Validate(rb.Quorum); e != nil {
			return e
		}
		seen := map[string]bool{}
		human, ai := 0, 0
		for _, id := range rb.Approvers {
			key := core.PublicKeyID(c.ApproverKeys[id])
			if key == "" || key == core.PublicKeyID(c.BrokerKey) || seen[key] {
				return errors.New("invalid, duplicate or Broker approval key")
			}
			seen[key] = true
			switch core.Kind(c.ApproverKinds[id]) {
			case "human":
				human++
			case "ai":
				ai++
			default:
				return errors.New("invalid approver kind")
			}
		}
		p := rb.Review.Effective()
		if p.Mode == "auto" && len(rb.Approvers) > 0 || p.Mode == "manual" && ai > 0 || p.Mode != "auto" && human < rb.Quorum || p.Mode == "ai" && ai < rb.Quorum {
			return errors.New("unreachable or invalid local review policy")
		}
		for _, requester := range rb.Requesters {
			eligible := 0
			for _, approver := range rb.Approvers {
				if approver != requester && core.Kind(c.ApproverKinds[approver]) == "human" {
					eligible++
				}
			}
			if p.Mode != "auto" && eligible < rb.Quorum {
				return errors.New("requester lacks independent human fallback")
			}
		}
	}
	return nil
}
