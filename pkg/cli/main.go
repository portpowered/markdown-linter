// Package cli exposes the stock command and the same command with a customer registry.
package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/portpowered/markdown-linter/pkg/engine"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"github.com/portpowered/markdown-linter/pkg/rulepack"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	exitOK          = 0
	exitViolation   = 1
	exitOperational = 2
)
const (
	ExitOK          = exitOK
	ExitViolation   = exitViolation
	ExitOperational = exitOperational
)

var Version = "dev"

func DiscoverMarkdownFiles(paths []string) ([]string, error) { return discoverMarkdownFiles(paths) }
func ParseMoveMappingFile(content []byte) (map[string]string, error) {
	return parseMoveMappingFile(content)
}
func RenderFixReport(out io.Writer, report engine.FixReviewReport, apply, verbose bool) {
	renderFixReport(out, report, apply, verbose)
}

// Run executes the stock registry, with Portos conventions available as an opt-in set.
func Run(ctx context.Context, args []string, out, errOut io.Writer) int {
	registry := rulepack.NewRegistry()
	if err := rulepack.RegisterStock(registry); err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return exitOperational
	}
	return RunWithRegistry(ctx, args, out, errOut, registry)
}

type commandOptions struct {
	pack, only, root, format, moveMap string
	baseline, baselineWrite, failOn   string
	baselineOverwrite                 bool
	fix, preview, verbose, version    bool
	moves                             moveMappingFlags
	paths                             []string
}

func parseOptions(args []string, stderr io.Writer) (commandOptions, error) {
	var opts commandOptions
	flags := flag.NewFlagSet("marklint", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.baseline, "baseline", "", "known findings file")
	flags.StringVar(&opts.baselineWrite, "baseline-write", "", "explicit baseline destination")
	flags.BoolVar(&opts.baselineOverwrite, "baseline-overwrite", false, "explicitly replace baseline")
	flags.StringVar(&opts.failOn, "fail-on", "error", "failure threshold: error or warning")
	flags.StringVar(&opts.pack, "rules", "", "YAML rule-pack path")
	flags.StringVar(&opts.only, "only", "", "comma-separated rule IDs from the selected pack")
	flags.StringVar(&opts.root, "root", ".", "document root; inputs must remain within it")
	flags.StringVar(&opts.format, "format", "text", "diagnostic format: text or json")
	flags.StringVar(&opts.moveMap, "move-map", "", "JSON or line-based move mapping file")
	flags.BoolVar(&opts.fix, "fix", false, "apply safe rule-provided fixes")
	flags.BoolVar(&opts.preview, "fix-check", false, "preview rule-provided fixes")
	flags.BoolVar(&opts.verbose, "fix-report-verbose", false, "show every non-fixable diagnostic")
	flags.BoolVar(&opts.version, "version", false, "print version")
	flags.Var(&opts.moves, "move", "old=new mapping for relocation checks; repeatable")
	if err := flags.Parse(args); err != nil {
		return opts, err
	}
	opts.paths = flags.Args()
	if opts.failOn != "error" && opts.failOn != "warning" {
		return opts, fmt.Errorf("fail-on must be error or warning")
	}
	if (opts.fix || opts.preview) && (opts.baseline != "" || opts.baselineWrite != "" || opts.format == "sarif") {
		return opts, fmt.Errorf("fix modes cannot be combined with baselines or SARIF")
	}
	if opts.baseline != "" && opts.baselineWrite != "" {
		return opts, fmt.Errorf("baseline read and write cannot be combined")
	}
	if opts.fix && opts.preview {
		return opts, fmt.Errorf("--fix and --fix-check cannot be combined")
	}
	if opts.format != "text" && opts.format != "json" && opts.format != "sarif" {
		return opts, fmt.Errorf("unknown format %q", opts.format)
	}
	return opts, nil
}

// RunWithRegistry allows customers to build a command with their own check factories.
func RunWithRegistry(ctx context.Context, args []string, out, errOut io.Writer, registry *rulepack.Registry) int {
	var baselineErr error
	args, baselineErr = rulepack.BaselineArgs(args)
	if baselineErr != nil {
		_, _ = fmt.Fprintln(errOut, baselineErr)
		return exitOperational
	}
	if handled, code := rulepack.Manage(args, ".marklint.yaml", registry, out, errOut); handled {
		return code
	}
	opts, err := parseOptions(args, errOut)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return exitOperational
	}
	if opts.version {
		if _, err := fmt.Fprintln(out, "marklint", Version); err != nil {
			return 2
		}
		return exitOK
	}
	if len(opts.paths) == 0 {
		_, _ = fmt.Fprintln(errOut, "at least one input file or directory is required")
		return exitOperational
	}
	pack, err := loadPack(opts)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return exitOperational
	}
	if err = registry.ValidateKind(pack, "markdown"); err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return exitOperational
	}
	if _, err = registry.Compile(pack); err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return exitOperational
	}
	pack, err = selectRules(pack, opts.only)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return exitOperational
	}
	program, err := registry.Compile(pack)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return exitOperational
	}
	docs, err := parseInputs(opts)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return exitOperational
	}
	diagnostics, err := program.Run(ctx, opts.root, docs)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return exitOperational
	}
	if opts.baselineWrite != "" {
		if err = rulepack.WriteBaseline(opts.baselineWrite, opts.root, diagnostics, opts.baselineOverwrite); err != nil {
			_, _ = fmt.Fprintln(errOut, err)
			return exitOperational
		}
		if _, err := fmt.Fprintf(out, "Baseline written to %s\n", opts.baselineWrite); err != nil {
			return 2
		}
		return exitOK
	}
	if opts.baseline != "" {
		var known int
		diagnostics, known, err = rulepack.ApplyBaseline(opts.baseline, opts.root, diagnostics)
		if err != nil {
			_, _ = fmt.Fprintln(errOut, err)
			return exitOperational
		}
		if _, err := fmt.Fprintf(errOut, "%d known findings in baseline\n", known); err != nil {
			return 2
		}
	}
	if opts.fix || opts.preview {
		return renderFixes(ctx, opts, diagnostics, out, errOut)
	}
	switch opts.format {
	case "json":
		err = json.NewEncoder(out).Encode(diagnostics)
	case "sarif":
		err = json.NewEncoder(out).Encode(rulepack.SARIF(diagnostics))
	default:
		for _, d := range diagnostics {
			if _, err = fmt.Fprintln(out, diagnosticString(d)); err != nil {
				break
			}
		}
	}
	if err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return exitOperational
	}
	for _, d := range diagnostics {
		if d.Severity == interfaces.SeverityError || (opts.failOn == "warning" && d.Severity == interfaces.SeverityWarning) {
			return exitViolation
		}
	}
	return exitOK
}

func loadPack(opts commandOptions) (rulepack.Pack, error) {
	pack := rulepack.DefaultPack()
	if opts.pack == "" {
		candidate := filepath.Join(opts.root, ".marklint.yaml")
		if _, err := os.Stat(candidate); err == nil {
			opts.pack = candidate
		} else if !os.IsNotExist(err) {
			return pack, err
		}
	}
	if opts.pack != "" {
		var err error
		pack, err = rulepack.Load(opts.pack, opts.root)
		if err != nil {
			return pack, err
		}
	}
	if len(opts.moves) > 0 || opts.moveMap != "" {
		moves, err := loadMoveMappings(opts.moves, opts.moveMap)
		if err != nil {
			return pack, err
		}
		found := false
		for i := range pack.Rules {
			if pack.Rules[i].Check == "markdown.link-relocation" {
				var config struct {
					Moves map[string]string `yaml:"moves"`
				}
				if err := rulepack.DecodeOptions(pack.Rules[i].Options, &config); err != nil {
					return pack, err
				}
				if config.Moves == nil {
					config.Moves = map[string]string{}
				}
				for old, newPath := range moves {
					config.Moves[old] = newPath
				}
				if err := pack.Rules[i].Options.Encode(config); err != nil {
					return pack, err
				}
				found = true
			}
		}
		if !found {
			return pack, fmt.Errorf("move mappings require a markdown.link-relocation check in the pack")
		}
	}
	return pack, nil
}

func selectRules(pack rulepack.Pack, only string) (rulepack.Pack, error) {
	if only == "" || only == "all" {
		return pack, nil
	}
	wanted := map[string]bool{}
	for _, id := range strings.Split(only, ",") {
		wanted[strings.TrimSpace(id)] = true
	}
	selected := []rulepack.Rule{}
	ids := map[string]bool{}
	for _, rule := range pack.Rules {
		if wanted[rule.ID] && (rule.Enabled == nil || *rule.Enabled) {
			selected = append(selected, rule)
			ids[rule.ID] = true
			delete(wanted, rule.ID)
		}
	}
	if len(wanted) > 0 {
		return pack, fmt.Errorf("--only contains a rule ID not present in the pack")
	}
	pack.Rules = selected
	suppressions := []rulepack.Suppression{}
	for _, s := range pack.Suppressions {
		if ids[s.Rule] {
			suppressions = append(suppressions, s)
		}
	}
	pack.Suppressions = suppressions
	return pack, nil
}

func parseInputs(opts commandOptions) ([]*interfaces.Document, error) {
	files, err := discoverMarkdownFiles(opts.paths)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no Markdown files found")
	}
	root, err := filepath.Abs(opts.root)
	if err != nil {
		return nil, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	docs := []*interfaces.Document{}
	linter := engine.New()
	for _, file := range files {
		resolved, err := filepath.EvalSymlinks(file)
		if err != nil {
			return nil, err
		}
		absolute, err := filepath.Abs(resolved)
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(root, absolute)
		if err != nil {
			return nil, err
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("input %q escapes document root", file)
		}
		doc, err := linter.ParseFile(file)
		if err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}
	return docs, nil
}

func renderFixes(ctx context.Context, opts commandOptions, diagnostics []interfaces.Diagnostic, out, errOut io.Writer) int {
	for _, diagnostic := range diagnostics {
		if err := interfaces.CheckPathRoot(opts.root, diagnostic.Path); err != nil {
			_, _ = fmt.Fprintf(errOut, "fix target %q: %v\n", diagnostic.Path, err)
			return exitOperational
		}
	}
	var plan engine.FixPlan
	var err error
	if opts.fix {
		plan, err = engine.ApplyFixes(ctx, diagnostics)
	} else {
		plan, err = engine.DryRunFixes(ctx, diagnostics)
	}
	if err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return exitOperational
	}
	report := engine.NewFixReviewReport(plan, diagnostics)
	if opts.format == "json" {
		err = json.NewEncoder(out).Encode(report)
	} else {
		tracked := &reportWriter{Writer: out}
		renderFixReport(tracked, report, opts.fix, opts.verbose)
		err = tracked.err
	}
	if err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return exitOperational
	}
	if opts.fix && plan.HasAcceptedEdits() {
		return exitOK
	}
	for _, d := range diagnostics {
		if d.Severity == interfaces.SeverityError || (opts.failOn == "warning" && d.Severity == interfaces.SeverityWarning) {
			return exitViolation
		}
	}
	return exitOK
}

func discoverMarkdownFiles(paths []string) ([]string, error) {
	seen := make(map[string]struct{})
	var files []string

	for _, path := range paths {
		cleanPath := filepath.Clean(path)
		info, err := os.Stat(cleanPath)
		if err != nil {
			return nil, fmt.Errorf("stat lint path %q: %w", path, err)
		}

		if !info.IsDir() {
			if isMarkdownFile(cleanPath) {
				if addUniqueFile(seen, cleanPath) {
					files = append(files, cleanPath)
				}
			}
			continue
		}

		err = filepath.WalkDir(cleanPath, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			if !isMarkdownFile(path) {
				return nil
			}
			if addUniqueFile(seen, path) {
				files = append(files, filepath.Clean(path))
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walk lint path %q: %w", path, err)
		}
	}

	sort.Strings(files)
	return files, nil
}

func addUniqueFile(seen map[string]struct{}, path string) bool {
	key := filepath.Clean(path)
	if _, exists := seen[key]; exists {
		return false
	}
	seen[key] = struct{}{}
	return true
}

func isMarkdownFile(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".md")
}

func diagnosticString(d interfaces.Diagnostic) string {
	return fmt.Sprintf("%s:%d: %s: %s: %s", d.Path, d.Line, d.Severity, d.RuleID, d.Message)
}

type moveMappingFlags map[string]string

func (m *moveMappingFlags) String() string {
	if m == nil || len(*m) == 0 {
		return ""
	}
	pairs := make([]string, 0, len(*m))
	for oldPath, newPath := range *m {
		pairs = append(pairs, oldPath+"="+newPath)
	}
	sort.Strings(pairs)
	return strings.Join(pairs, ",")
}

func (m *moveMappingFlags) Set(value string) error {
	oldPath, newPath, ok := strings.Cut(value, "=")
	if !ok || strings.TrimSpace(oldPath) == "" || strings.TrimSpace(newPath) == "" {
		return fmt.Errorf("move mapping must use old=new format: %q", value)
	}
	if *m == nil {
		*m = map[string]string{}
	}
	(*m)[normalizeMappingPath(oldPath)] = normalizeMappingPath(newPath)
	return nil
}

func loadMoveMappings(flags moveMappingFlags, mappingPath string) (map[string]string, error) {
	mappings := map[string]string{}
	for oldPath, newPath := range flags {
		mappings[oldPath] = newPath
	}
	if mappingPath == "" {
		return mappings, nil
	}

	content, err := os.ReadFile(mappingPath)
	if err != nil {
		return nil, fmt.Errorf("reading move mapping file %q: %w", mappingPath, err)
	}
	fileMappings, err := parseMoveMappingFile(content)
	if err != nil {
		return nil, fmt.Errorf("parsing move mapping file %q: %w", mappingPath, err)
	}
	for oldPath, newPath := range fileMappings {
		mappings[oldPath] = newPath
	}
	return mappings, nil
}

func parseMoveMappingFile(content []byte) (map[string]string, error) {
	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		return map[string]string{}, nil
	}
	if strings.HasPrefix(trimmed, "{") {
		var parsed map[string]string
		if err := json.Unmarshal(content, &parsed); err != nil {
			return nil, err
		}
		return normalizeMoveMappings(parsed), nil
	}

	mappings := map[string]string{}
	for index, line := range strings.Split(strings.ReplaceAll(trimmed, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		oldPath, newPath, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(oldPath) == "" || strings.TrimSpace(newPath) == "" {
			return nil, fmt.Errorf("line %d must use old=new format", index+1)
		}
		mappings[normalizeMappingPath(oldPath)] = normalizeMappingPath(newPath)
	}
	return mappings, nil
}

func normalizeMoveMappings(source map[string]string) map[string]string {
	mappings := map[string]string{}
	for oldPath, newPath := range source {
		if strings.TrimSpace(oldPath) == "" || strings.TrimSpace(newPath) == "" {
			continue
		}
		mappings[normalizeMappingPath(oldPath)] = normalizeMappingPath(newPath)
	}
	return mappings
}

func normalizeMappingPath(path string) string {
	return filepath.ToSlash(filepath.Clean(strings.ReplaceAll(strings.TrimSpace(path), `\`, "/")))
}

func renderFixReport(stdout io.Writer, report engine.FixReviewReport, apply bool, verboseReport bool) {
	if apply {
		_, _ = fmt.Fprintln(stdout, "Markdown linter apply report")
	} else {
		_, _ = fmt.Fprintln(stdout, "Markdown linter dry-run report")
	}

	if len(report.Accepted) == 0 {
		_, _ = fmt.Fprintln(stdout, "Accepted edits: none")
	} else {
		_, _ = fmt.Fprintf(stdout, "Accepted edits: %d\n", len(report.Accepted))
		for _, edit := range report.Accepted {
			action := "would rewrite"
			if apply {
				action = "rewrote"
			}
			_, _ = fmt.Fprintf(stdout, "- %s:%d: %s %s -> %q\n", edit.Path, edit.Diagnostic.Line, edit.RuleID, action, edit.TextEdit.Replacement)
		}
	}

	renderManualReviewSummary(stdout, report, verboseReport)

	if apply {
		_, _ = fmt.Fprintln(stdout, "Verification: rerun lint with the same rule pack and inputs")
		return
	}
	_, _ = fmt.Fprintln(stdout, "Apply safe fixes: rerun with --fix and the same rule pack and inputs")
}

func renderManualReviewSummary(stdout io.Writer, report engine.FixReviewReport, verboseReport bool) {
	renderRejectedSummary(stdout, report.Rejected, verboseReport)
	renderDiagnosticSummary(stdout, "Ambiguous cases", report.Ambiguous, verboseReport)
	renderDiagnosticSummary(stdout, "Other non-fixable diagnostics", report.OtherNonFixable, verboseReport)

	if !verboseReport && hasManualReviewDiagnostics(report) {
		_, _ = fmt.Fprintln(stdout, "Full manual-review details: rerun with --fix-report-verbose")
	}
}

func renderRejectedSummary(stdout io.Writer, rejected []engine.RejectedFix, verboseReport bool) {
	if len(rejected) == 0 {
		_, _ = fmt.Fprintln(stdout, "Rejected edits: none")
		return
	}

	_, _ = fmt.Fprintf(stdout, "Rejected edits: %d\n", len(rejected))
	if !verboseReport {
		return
	}
	for _, fix := range rejected {
		_, _ = fmt.Fprintf(stdout, "- %s:%d: %s (%s): %s [rejected: %s - %s]\n", fix.Path, fix.Diagnostic.Line, fix.RuleID, fix.Diagnostic.Category, fix.Diagnostic.Message, fix.Reason, fix.Message)
	}
}

func renderDiagnosticSummary(stdout io.Writer, title string, diagnostics []interfaces.Diagnostic, verboseReport bool) {
	if len(diagnostics) == 0 {
		_, _ = fmt.Fprintf(stdout, "%s: none\n", title)
		return
	}

	_, _ = fmt.Fprintf(stdout, "%s: %d\n", title, len(diagnostics))
	if !verboseReport {
		return
	}
	for _, diagnostic := range diagnostics {
		_, _ = fmt.Fprintf(stdout, "- %s:%d: %s (%s): %s\n", diagnostic.Path, diagnostic.Line, diagnostic.RuleID, diagnostic.Category, diagnostic.Message)
	}
}

func hasManualReviewDiagnostics(report engine.FixReviewReport) bool {
	return len(report.Rejected) > 0 || len(report.Ambiguous) > 0 || len(report.OtherNonFixable) > 0
}

// reportWriter preserves failures from the legacy void report-rendering API.
type reportWriter struct {
	io.Writer
	err error
}

func (w *reportWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	n, err := w.Writer.Write(p)
	if err == nil && n < len(p) {
		err = io.ErrShortWrite
	}
	w.err = err
	return n, err
}
