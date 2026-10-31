package core

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Diagnostic operations are built-in file readers, never shell commands.
// Roots are deliberate disclosure boundaries installed by a trusted admin.
type DiagnosticRoot struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}
type DiagnosticPolicy struct {
	Roots     []DiagnosticRoot `json:"roots"`
	Tools     []string         `json:"tools"`
	AutoTools []string         `json:"auto_tools,omitempty"`
}
type DiagnosticOperation struct {
	Tool     string `json:"tool"`
	Resource string `json:"resource,omitempty"`
	Path     string `json:"path,omitempty"`
	Query    string `json:"query,omitempty"`
	Lines    int    `json:"lines,omitempty"`
}
type ChangeStep struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Argv        []string `json:"argv"`
	Timeout     int      `json:"timeout_seconds"`
}
type PlanPolicy struct {
	Steps      []ChangeStep `json:"steps"`
	MaxSteps   int          `json:"max_steps"`
	Privileged bool         `json:"privileged"`
}

func (rb Runbook) Kind() string {
	if rb.Diagnostics != nil {
		return "session"
	}
	if rb.Plan != nil {
		return "plan"
	}
	return "task"
}
func (a Action) RunbookSpec() Runbook {
	return Runbook{Timeout: a.Timeout, Review: a.Review, Quorum: a.Quorum, Diagnostics: a.Diagnostics, Plan: a.Plan}
}
func (rb Runbook) Hash() string {
	if rb.Kind() == "task" {
		return CommandHash(rb.Argv, rb.Timeout)
	}
	return Hash(JSON(struct {
		Kind        string
		Timeout     int
		Diagnostics *DiagnosticPolicy
		Plan        *PlanPolicy
	}{rb.Kind(), rb.Timeout, rb.Diagnostics, rb.Plan}))
}
func safeText(s string, max int) bool {
	return len(s) <= max && utf8.ValidString(s) && !strings.ContainsFunc(s, func(r rune) bool { return unicode.IsControl(r) })
}
func (d DiagnosticPolicy) Validate() error {
	if len(d.Roots) > 8 || len(d.Tools) == 0 || len(d.Tools) > 5 {
		return errors.New("diagnostic profile must bound tools and roots")
	}
	seen := map[string]bool{}
	for _, r := range d.Roots {
		if !ValidWorkspaceID(r.ID) || seen[r.ID] || !filepath.IsAbs(r.Path) || filepath.Clean(r.Path) != r.Path || r.Path == "/" || !safeText(r.Path, 1024) {
			return errors.New("invalid diagnostic root; never expose the filesystem root")
		}
		seen[r.ID] = true
	}
	seen = map[string]bool{}
	for _, tool := range d.Tools {
		if seen[tool] || !slices.Contains([]string{"ls", "cat", "tail", "grep", "status"}, tool) {
			return errors.New("unknown or repeated diagnostic tool")
		}
		seen[tool] = true
		if tool != "status" && len(d.Roots) == 0 {
			return errors.New("file tools require explicit roots")
		}
	}
	for _, tool := range d.AutoTools {
		if !slices.Contains([]string{"ls", "status"}, tool) || !slices.Contains(d.Tools, tool) {
			return errors.New("only scoped ls/status may bypass review")
		}
	}
	return nil
}
func (d DiagnosticPolicy) ValidateOperation(o DiagnosticOperation) error {
	if d.Validate() != nil || !slices.Contains(d.Tools, o.Tool) {
		return errors.New("tool outside approved session scope")
	}
	if o.Tool == "status" {
		if o.Resource != "" || o.Path != "" || o.Query != "" || o.Lines != 0 {
			return errors.New("status takes no arguments")
		}
		return nil
	}
	if !slices.ContainsFunc(d.Roots, func(r DiagnosticRoot) bool { return r.ID == o.Resource }) {
		return errors.New("resource outside approved session scope")
	}
	// Require canonical, relative slash-separated paths; do not silently clean.
	if o.Path == "" || !safeText(o.Path, 1024) || strings.Contains(o.Path, "\\") || path.IsAbs(o.Path) || path.Clean(o.Path) != o.Path || o.Path == ".." || strings.HasPrefix(o.Path, "../") {
		return errors.New("path must be canonical and relative to the approved resource")
	}
	if o.Tool != "ls" && o.Path == "." {
		return errors.New("select a regular file")
	}
	if o.Tool == "grep" {
		if strings.TrimSpace(o.Query) == "" || !safeText(o.Query, 200) {
			return errors.New("search text must contain 1–200 characters")
		}
	} else if o.Query != "" {
		return errors.New("only grep accepts search text")
	}
	if o.Tool == "tail" {
		if o.Lines < 1 || o.Lines > 200 {
			return errors.New("tail allows 1–200 lines")
		}
	} else if o.Lines != 0 {
		return errors.New("only tail accepts a line count")
	}
	return nil
}

// ValidateSpec also validates catalog metadata, where legacy task argv remains
// private to the Connector. Review mode cannot be selected by request input.
func (rb Runbook) ValidateSpec(catalog bool) error {
	if rb.Diagnostics != nil && rb.Plan != nil {
		return errors.New("task cannot be both a session and a plan")
	}
	if e := rb.Review.Validate(rb.Quorum); e != nil {
		return e
	}
	p := rb.Review.Effective()
	switch rb.Kind() {
	case "session":
		if len(rb.Argv) != 0 || rb.Timeout < 1 || rb.Timeout > 30 || p.Mode == "auto" || p.Risk == "high" {
			return errors.New("sessions require bounded built-in tools and human/AI review")
		}
		return rb.Diagnostics.Validate()
	case "plan":
		if len(rb.Argv) != 0 || rb.Timeout < 1 || rb.Timeout > 600 || p.Mode != "manual" || p.Risk != "high" || p.MaxUses != 1 {
			return errors.New("change plans require manual high-risk single-use approval")
		}
		if rb.Plan.Privileged && rb.Quorum < 2 {
			return errors.New("privileged plans require two independent humans")
		}
		if rb.Plan.MaxSteps < 1 || rb.Plan.MaxSteps > 8 || len(rb.Plan.Steps) < 1 || len(rb.Plan.Steps) > 24 {
			return errors.New("invalid plan step limits")
		}
		seen := map[string]bool{}
		for _, s := range rb.Plan.Steps {
			if !ValidWorkspaceID(s.ID) || seen[s.ID] || s.Name == "" || !safeText(s.Name, 120) || !safeText(s.Description, 500) || len(s.Argv) == 0 || len(s.Argv) > 24 || !filepath.IsAbs(s.Argv[0]) || s.Timeout < 1 || s.Timeout > 600 {
				return errors.New("invalid installed change step")
			}
			seen[s.ID] = true
			size := 0
			for _, arg := range s.Argv {
				if !safeText(arg, 1024) {
					return errors.New("invalid change argument")
				}
				size += len(arg)
			}
			if size > 2048 {
				return errors.New("change arguments too large")
			}
		}
	default:
		if rb.Timeout < 1 || rb.Timeout > 120 || p.MaxUses > 20 || (!catalog && (len(rb.Argv) == 0 || !filepath.IsAbs(rb.Argv[0]))) {
			return errors.New("invalid fixed task")
		}
	}
	return nil
}
func (p PlanPolicy) Select(ids []string, timeout int) ([]ChangeStep, error) {
	if len(ids) == 0 || len(ids) > p.MaxSteps {
		return nil, errors.New("select 1..max_steps installed changes")
	}
	out := []ChangeStep{}
	seen := map[string]bool{}
	total := 0
	for _, id := range ids {
		n := slices.IndexFunc(p.Steps, func(s ChangeStep) bool { return s.ID == id })
		if n < 0 || seen[id] {
			return nil, errors.New("unknown or repeated change step")
		}
		seen[id] = true
		s := p.Steps[n]
		out = append(out, s)
		total += s.Timeout
	}
	if total > timeout {
		return nil, errors.New("selected steps exceed the plan time budget")
	}
	return out, nil
}
func (rb Runbook) ValidateIntent(i Intent) error {
	if rb.Kind() == "task" {
		if i.Kind != "" || i.Scope != nil || len(i.Plan) > 0 || i.Operation != nil {
			return errors.New("unexpected workflow fields")
		}
		return nil
	}
	if i.Version != 4 || (i.Kind != rb.Kind() && !(rb.Kind() == "session" && i.Kind == "command")) {
		return errors.New("workflow requires scoped intent v4")
	}
	if rb.Kind() == "session" {
		if i.Kind == "command" {
			if i.Operation == nil || Uses(i) != 1 || rb.Diagnostics.ValidateOperation(*i.Operation) != nil {
				return errors.New("invalid exact command")
			}
		} else if i.Operation != nil {
			return errors.New("session scope cannot contain command")
		}
		if i.Scope == nil || len(i.Plan) > 0 || string(JSON(i.Scope)) != string(JSON(rb.Diagnostics)) {
			return errors.New("session scope changed")
		}
		return nil
	}
	if i.Scope != nil || i.Operation != nil || Uses(i) != 1 {
		return errors.New("invalid change-plan scope")
	}
	ids := []string{}
	for _, step := range i.Plan {
		ids = append(ids, step.ID)
	}
	selected, e := rb.Plan.Select(ids, rb.Timeout)
	if e != nil {
		return e
	}
	if string(JSON(selected)) != string(JSON(i.Plan)) {
		return errors.New("approved plan differs from locally installed changes")
	}
	return nil
}
func OperationLabel(o *DiagnosticOperation) string {
	if o == nil {
		return ""
	}
	if o.Tool == "status" {
		return "status"
	}
	s := o.Tool + " " + o.Resource + ":" + o.Path
	if o.Query != "" {
		s += " " + o.Query
	}
	if o.Lines > 0 {
		s += fmt.Sprintf(" %d", o.Lines)
	}
	return s
}

// Only locally pinned diagnostic tools can take the rule fast path.
func (rb Runbook) PolicyFor(i Intent) ReviewPolicy {
	p := rb.Review.Effective()
	if rb.Diagnostics != nil && i.Kind == "command" && i.Operation != nil && rb.ValidateIntent(i) == nil && slices.Contains(rb.Diagnostics.AutoTools, i.Operation.Tool) {
		return ReviewPolicy{Mode: "auto", Risk: "low", MaxUses: 1, RateLimit: p.RateLimit}
	}
	return p
}
func (rb Runbook) QuorumFor(i Intent) int {
	if rb.PolicyFor(i).Mode == "auto" {
		return 0
	}
	return rb.Quorum
}
