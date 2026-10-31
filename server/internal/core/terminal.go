package core

import (
	"errors"
	"path"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// Words is deliberately not a shell grammar. No expansion, pipelines, redirects,
// control operators, command substitution or background execution is accepted.
func Words(line string) ([]string, error) {
	if !safeText(line, 2048) || strings.ContainsAny(line, "`$;|&<>\\") {
		return nil, errors.New("shell expansion, pipes, redirects and control operators are not supported")
	}
	words := []string{}
	var word strings.Builder
	var quote rune
	started := false
	for _, r := range line {
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				word.WriteRune(r)
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			started = true
			continue
		}
		if unicode.IsSpace(r) {
			if started {
				words = append(words, word.String())
				word.Reset()
				started = false
			}
			continue
		}
		started = true
		word.WriteRune(r)
	}
	if quote != 0 {
		return nil, errors.New("unclosed quote")
	}
	if started {
		words = append(words, word.String())
	}
	if len(words) == 0 || len(words) > 24 {
		return nil, errors.New("enter one bounded command")
	}
	return words, nil
}
func (p DiagnosticPolicy) Parse(line string) (DiagnosticOperation, error) {
	words, err := Words(line)
	if err != nil {
		return DiagnosticOperation{}, err
	}
	o := DiagnosticOperation{Tool: words[0]}
	args := words[1:]
	if o.Tool == "status" {
		if len(args) != 0 {
			return o, errors.New("status takes no arguments")
		}
		return o, p.ValidateOperation(o)
	}
	if !slices.Contains(p.Tools, o.Tool) {
		return o, errors.New("command is outside the installed diagnostic profile; use run <task> or plan <plan> <step...>")
	}
	if o.Tool == "ls" && len(args) > 0 && slices.Contains([]string{"-l", "-a", "-la", "-al"}, args[0]) {
		args = args[1:]
	}
	if o.Tool == "tail" {
		o.Lines = 50
		if len(args) >= 2 && args[0] == "-n" {
			o.Lines, err = strconv.Atoi(args[1])
			if err != nil {
				return o, errors.New("invalid tail line count")
			}
			args = args[2:]
		}
	}
	if o.Tool == "grep" {
		if len(args) < 2 {
			return o, errors.New("usage: grep 'literal text' resource:path")
		}
		o.Query = args[0]
		args = args[1:]
	}
	if len(args) == 0 && o.Tool == "ls" {
		args = []string{"."}
	}
	if len(args) != 1 || len(p.Roots) == 0 {
		return o, errors.New("select one path in an approved resource")
	}
	value := args[0]
	o.Resource = p.Roots[0].ID
	o.Path = value
	if path.IsAbs(value) {
		found := false
		for _, root := range p.Roots {
			if value == root.Path {
				o.Resource = root.ID
				o.Path = "."
				found = true
				break
			}
			if strings.HasPrefix(value, root.Path+"/") {
				o.Resource = root.ID
				o.Path = strings.TrimPrefix(value, root.Path+"/")
				found = true
				break
			}
		}
		if !found {
			return o, errors.New("absolute path is outside approved resources")
		}
	} else if resource, relative, ok := strings.Cut(value, ":"); ok {
		o.Resource = resource
		o.Path = relative
	}
	if o.Path == "" {
		o.Path = "."
	}
	return o, p.ValidateOperation(o)
}
