package core

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// RiskContext is a bounded, immutable snapshot, not an instruction source.
// Only records for this requester + workspace + project + target are selected.
type RiskContext struct {
	ManualResetAt int64        `json:"manual_reset_at,omitempty"`
	Version       int          `json:"version"`
	CapturedAt    int64        `json:"captured_at"`
	Since         int64        `json:"since"`
	Records       []RiskRecord `json:"records"`
	Truncated     bool         `json:"truncated"`
}
type RiskRecord struct {
	DeviceID  string `json:"device_id,omitempty"`
	RequestID string `json:"request_id"`
	Action    string `json:"action"`
	Operation string `json:"operation,omitempty"`
	Status    string `json:"status"`
	At        int64  `json:"at"`
	ExitCode  *int   `json:"exit_code,omitempty"`
	Output    string `json:"output,omitempty"`
}

func (c *RiskContext) Digest() string {
	if c == nil {
		return ""
	}
	return Hash(JSON(c))
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?(?:-----END [A-Z ]*PRIVATE KEY-----|$)`),
	regexp.MustCompile(`(?im)(?:authorization\s*[:=]\s*|bearer\s+)[^\r\n]+`),
	regexp.MustCompile(`(?i)(?:["']?[\w.-]*(?:password|passwd|secret|token|api[_-]?key)[\w.-]*["']?\s*[:=]\s*)(?:"[^"\r\n]*"|'[^'\r\n]*'|[^\s,;]+)`),
	regexp.MustCompile(`(?:gh[pousr]_[A-Za-z0-9_]{16,}|github_pat_[A-Za-z0-9_]{16,}|sk-[A-Za-z0-9_-]{16,}|AKIA[A-Z0-9]{16})`),
	regexp.MustCompile(`(?i)(?:https?|postgres(?:ql)?|mysql|redis)://[^\s/@:]+:[^\s/@]+@`),
}

func Clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

// Redact before clipping, so truncated credentials do not bypass matching.
// This is best-effort; roots must exclude credentials even with redaction.
func Redact(s string, n int) string {
	s = strings.ToValidUTF8(s, "")
	for _, p := range secretPatterns {
		s = p.ReplaceAllString(s, "[REDACTED]")
	}
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, s)
	if len(s) > n {
		return Clip(s, n-24) + "\n[context truncated]"
	}
	return s
}
func CollectRiskContext(st *State, i Intent, now int64) *RiskContext {
	c := &RiskContext{Version: 1, CapturedAt: now, Since: now - 86400, Records: []RiskRecord{}}
	for _, lock := range st.AccountLocks {
		if lock.Requester == i.Requester && lock.ReleasedAt > c.Since {
			c.Since = lock.ReleasedAt
			c.ManualResetAt = lock.ReleasedAt
		}
	}
	for _, r := range st.Requests {
		ri := r.Intent
		if ri.ID == i.ID || ri.Requester != i.Requester || ri.WorkspaceID != i.WorkspaceID || ri.ProjectID != i.ProjectID || ri.Target != i.Target {
			continue
		}
		for _, e := range r.Executions {
			if e.StartedAt < c.Since || c.ManualResetAt > 0 && e.StartedAt <= c.ManualResetAt || e.StartedAt > now {
				continue
			}
			c.Records = append(c.Records, RiskRecord{DeviceID: r.DeviceID, RequestID: ri.ID, Action: ri.Action, Operation: Redact(OperationLabel(e.Operation), 240), Status: e.Status, At: e.StartedAt, ExitCode: e.ExitCode, Output: Redact(e.Output, 1024)})
		}
		if len(r.Executions) == 0 && ri.CreatedAt >= c.Since && (c.ManualResetAt == 0 || ri.CreatedAt > c.ManualResetAt) && ri.CreatedAt <= now {
			c.Records = append(c.Records, RiskRecord{DeviceID: r.DeviceID, RequestID: ri.ID, Action: ri.Action, Operation: Redact(OperationLabel(ri.Operation), 240), Status: r.Status, At: ri.CreatedAt})
		}
	}
	sort.SliceStable(c.Records, func(a, b int) bool { return c.Records[a].At > c.Records[b].At })
	if len(c.Records) > 10 {
		c.Records = c.Records[:10]
		c.Truncated = true
	}
	budget := 8192
	for n := range c.Records {
		v := &c.Records[n]
		if len(v.Output) > budget {
			v.Output = Clip(v.Output, budget)
			c.Truncated = true
		}
		budget -= len(v.Output)
	}
	return c
}
