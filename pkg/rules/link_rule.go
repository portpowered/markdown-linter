package rules

import (
	"context"
	"fmt"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

const localLinkRuleID = "markdown.link"

// LocalLinkRule validates repository-local Markdown file and heading links.
type LocalLinkRule struct {
	markdown              goldmark.Markdown
	allowDirectoryTargets bool
	root                  string
}

var _ interfaces.Rule = (*LocalLinkRule)(nil)

// LocalLinkRuleOption configures local-link validation.
type LocalLinkRuleOption func(*LocalLinkRule)

// WithDirectoryLinkTargetsAllowed allows local links to existing directories.
func WithDirectoryLinkTargetsAllowed() LocalLinkRuleOption {
	return func(rule *LocalLinkRule) {
		rule.allowDirectoryTargets = true
	}
}

// WithLinkRoot restricts local link targets to the specified document root.
func WithLinkRoot(root string) LocalLinkRuleOption {
	return func(rule *LocalLinkRule) { rule.root = root }
}

// NewLocalLinkRule creates a rule for local Markdown links.
func NewLocalLinkRule(opts ...LocalLinkRuleOption) *LocalLinkRule {
	rule := &LocalLinkRule{
		markdown: goldmark.New(),
	}
	for _, opt := range opts {
		opt(rule)
	}
	return rule
}

// ID returns the stable local-link rule identifier.
func (r *LocalLinkRule) ID() string {
	return localLinkRuleID
}

// Check validates relative file and heading links in a parsed Markdown document.
func (r *LocalLinkRule) Check(ctx context.Context, doc *interfaces.Document) []interfaces.Violation {
	var violations []interfaces.Violation

	err := doc.Walk(func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if err := ctx.Err(); err != nil {
			return ast.WalkStop, err
		}

		switch link := node.(type) {
		case *ast.Link:
			violations = append(violations, r.checkLink(doc, string(link.Destination), doc.NodeLine(link))...)
		case *ast.Image:
			violations = append(violations, r.checkLink(doc, string(link.Destination), doc.NodeLine(link))...)
		default:
			return ast.WalkContinue, nil
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		violations = append(violations, interfaces.NewViolation(doc.Path, 0, r.ID(), err.Error()))
	}

	return violations
}

func (r *LocalLinkRule) checkLink(doc *interfaces.Document, destination string, line int) []interfaces.Violation {
	rawDestination := strings.TrimSpace(destination)
	target, ok := parseLocalLinkTarget(rawDestination)
	if !ok {
		return nil
	}

	targetPath := doc.Path
	displayTarget := rawDestination
	if target.path != "" {
		targetPath = filepath.Clean(filepath.Join(filepath.Dir(doc.Path), filepath.FromSlash(target.path)))
		displayTarget = target.path
	}

	if err := interfaces.CheckPathRoot(r.root, targetPath); err != nil {
		return []interfaces.Violation{interfaces.NewViolation(doc.Path, line, r.ID(), fmt.Sprintf("local link target is outside document root or inaccessible: %s", displayTarget))}
	}

	if target.path != "" {
		info, err := os.Stat(targetPath)
		if err != nil {
			return []interfaces.Violation{interfaces.NewViolation(
				doc.Path,
				line,
				r.ID(),
				fmt.Sprintf("local link target does not exist: %s", displayTarget),
			)}
		}
		if info.IsDir() && !r.allowDirectoryTargets {
			return []interfaces.Violation{interfaces.NewViolation(
				doc.Path,
				line,
				r.ID(),
				fmt.Sprintf("local link target is a directory, not a file: %s", displayTarget),
			)}
		}
	}

	if target.anchor == "" {
		return nil
	}

	anchors, err := r.anchorsForTarget(doc, targetPath)
	if err != nil {
		return []interfaces.Violation{interfaces.NewViolation(
			doc.Path,
			line,
			r.ID(),
			fmt.Sprintf("local link target could not be parsed for anchors: %s", displayTarget),
		)}
	}

	if _, exists := anchors[target.anchor]; !exists {
		return []interfaces.Violation{interfaces.NewViolation(
			doc.Path,
			line,
			r.ID(),
			fmt.Sprintf("local link anchor does not exist: %s", rawDestination),
		)}
	}

	return nil
}

func (r *LocalLinkRule) anchorsForTarget(doc *interfaces.Document, targetPath string) (map[string]struct{}, error) {
	if filepath.Clean(targetPath) == filepath.Clean(doc.Path) {
		return collectHeadingAnchors(doc.Root, doc.Source), nil
	}

	source, err := os.ReadFile(targetPath)
	if err != nil {
		return nil, err
	}

	root := r.markdown.Parser().Parse(text.NewReader(source))
	return collectHeadingAnchors(root, source), nil
}

type localLinkTarget struct {
	path   string
	anchor string
}

func parseLocalLinkTarget(rawDestination string) (localLinkTarget, bool) {
	if rawDestination == "" {
		return localLinkTarget{}, false
	}

	parsed, err := url.Parse(rawDestination)
	if err != nil {
		return localLinkTarget{}, false
	}

	if parsed.IsAbs() || parsed.Scheme != "" || parsed.Host != "" {
		return localLinkTarget{}, false
	}
	if strings.HasPrefix(rawDestination, "//") || strings.HasPrefix(parsed.Path, "/") {
		return localLinkTarget{}, false
	}

	anchor, err := url.PathUnescape(parsed.Fragment)
	if err != nil {
		anchor = parsed.Fragment
	}

	path, err := url.PathUnescape(parsed.Path)
	if err != nil {
		path = parsed.Path
	}

	return localLinkTarget{
		path:   path,
		anchor: normalizeAnchor(anchor),
	}, true
}

func collectHeadingAnchors(root ast.Node, source []byte) map[string]struct{} {
	anchors := make(map[string]struct{})
	seen := make(map[string]int)

	_ = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || node.Kind() != ast.KindHeading {
			return ast.WalkContinue, nil
		}

		headingText := string(interfaces.NodeText(node, source))
		baseAnchor := normalizeAnchor(headingText)
		if baseAnchor == "" {
			return ast.WalkContinue, nil
		}

		index := seen[baseAnchor]
		seen[baseAnchor] = index + 1

		anchor := baseAnchor
		if index > 0 {
			anchor = fmt.Sprintf("%s-%d", baseAnchor, index)
		}
		anchors[anchor] = struct{}{}

		return ast.WalkContinue, nil
	})

	return anchors
}

func normalizeAnchor(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))

	var builder strings.Builder
	lastWasHyphen := false
	for _, char := range value {
		switch {
		case unicode.IsLetter(char) || unicode.IsDigit(char):
			builder.WriteRune(char)
			lastWasHyphen = false
		case unicode.IsSpace(char) || char == '-':
			if builder.Len() > 0 && !lastWasHyphen {
				builder.WriteByte('-')
				lastWasHyphen = true
			}
		}
	}

	return strings.Trim(builder.String(), "-")
}
