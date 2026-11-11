// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Go templates, as GoReleaser uses them, with the fields that
// the release command has data for.

package spec

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"text/template"
	"text/template/parse"
)

// Template is the data of a Go template, as GoReleaser renders it: the
// fields that the release command supports. .Env comes from the caller.
type Template struct {
	Version string
	Tag     string
}

// Apply renders a template with .Env from env. It rejects a
// template that parseTemplate rejects with ErrUnsupported. A missing key
// is an error, as in GoReleaser.
func (t *Template) Apply(s string, env []string) (string, error) {
	tmpl, err := parseTemplate(s)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	data := map[string]any{"Version": t.Version, "Tag": t.Tag, "Env": EnvMap(env)}
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	return buf.String(), nil
}

// EnvMap returns the NAME=value items of env as a map, which is .Env in
// a template. Later items win, and items with no name are left out, as
// in GoReleaser.
func EnvMap(env []string) map[string]string {
	m := map[string]string{}
	for _, e := range env {
		if k, v, ok := strings.Cut(e, "="); ok && k != "" {
			m[k] = v
		}
	}
	return m
}

// CheckTemplate rejects a template that the release command cannot
// render as GoReleaser does.
func CheckTemplate(s string) error {
	_, err := parseTemplate(s)
	return err
}

// parseTemplate parses a template as GoReleaser does, and rejects it
// unless it uses only the fields .Version, .Tag, .Env and .Env.<NAME>,
// with text, variables, dot, literals, pipelines, and if, range and with.
// GoReleaser has more fields and functions, which the release command
// does not have the data for.
func parseTemplate(s string) (*template.Template, error) {
	tmpl, err := template.New("tmpl").Option("missingkey=error").Parse(s)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnsupported, err)
	}
	if len(tmpl.Templates()) != 1 {
		return nil, fmt.Errorf("%w: define in a template", ErrUnsupported)
	}
	if err := checkTemplateNode(tmpl.Root); err != nil {
		return nil, err
	}
	return tmpl, nil
}

// checkTemplateNode returns an error that names the first node below n
// that parseTemplate does not allow.
func checkTemplateNode(n parse.Node) error {
	switch n := n.(type) {
	case *parse.ListNode:
		if n == nil {
			return nil
		}
		for _, c := range n.Nodes {
			if err := checkTemplateNode(c); err != nil {
				return err
			}
		}
	case *parse.ActionNode:
		return checkTemplateNode(n.Pipe)
	case *parse.IfNode:
		return checkBranchNode(&n.BranchNode)
	case *parse.RangeNode:
		return checkBranchNode(&n.BranchNode)
	case *parse.WithNode:
		return checkBranchNode(&n.BranchNode)
	case *parse.PipeNode:
		for _, c := range n.Cmds {
			if err := checkTemplateNode(c); err != nil {
				return err
			}
		}
	case *parse.CommandNode:
		for _, a := range n.Args {
			if err := checkTemplateNode(a); err != nil {
				return err
			}
		}
	case *parse.FieldNode:
		return checkTemplateField(n.Ident)
	case *parse.VariableNode:
		// Only $ has the template fields.
		if len(n.Ident) > 1 && n.Ident[0] != "$" {
			return fmt.Errorf("%w: field %s", ErrUnsupported, n)
		}
		if len(n.Ident) > 1 {
			return checkTemplateField(n.Ident[1:])
		}
	case *parse.IdentifierNode:
		return fmt.Errorf("%w: function %q", ErrUnsupported, n.Ident)
	case *parse.TextNode, *parse.CommentNode, *parse.DotNode, *parse.StringNode, *parse.NumberNode,
		*parse.BoolNode, *parse.NilNode, *parse.BreakNode, *parse.ContinueNode:
	default:
		return fmt.Errorf("%w: %q", ErrUnsupported, n)
	}
	return nil
}

// checkBranchNode checks the pipeline and both lists of an if, range or
// with node.
func checkBranchNode(n *parse.BranchNode) error {
	for _, c := range []parse.Node{n.Pipe, n.List, n.ElseList} {
		if err := checkTemplateNode(c); err != nil {
			return err
		}
	}
	return nil
}

// checkTemplateField allows the fields .Version, .Tag, .Env and
// .Env.<NAME>.
func checkTemplateField(ident []string) error {
	switch {
	case len(ident) == 1 && slices.Contains([]string{"Version", "Tag", "Env"}, ident[0]),
		len(ident) == 2 && ident[0] == "Env":
		return nil
	}
	return fmt.Errorf("%w: field .%s", ErrUnsupported, strings.Join(ident, "."))
}
