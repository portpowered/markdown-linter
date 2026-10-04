package contract

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/portpowered/markdown-linter/pkg/engine"
	"github.com/portpowered/markdown-linter/pkg/interfaces"
	"github.com/portpowered/markdown-linter/pkg/rulepack"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

type repeated []string

func (r *repeated) String() string     { return strings.Join(*r, ",") }
func (r *repeated) Set(s string) error { *r = append(*r, s); return nil }
func Run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer, registry *rulepack.Registry, version string) int {
	return RunWithCapabilities(ctx, args, in, out, errOut, registry, NewCapabilities(), version)
}

// RunWithCapabilities exposes operator-installed measures and provider profiles to a custom command.
func RunWithCapabilities(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer, registry *rulepack.Registry, capabilities *Capabilities, version string) int {
	if capabilities == nil {
		capabilities = NewCapabilities()
	}
	command := "lint"
	if len(args) > 1 && (args[0] == "config" || args[0] == "sets" || args[0] == "rules") {
		command = args[0] + " " + args[1]
		args = args[2:]
	}
	flags := flag.NewFlagSet("marklint", flag.ContinueOnError)
	flags.SetOutput(errOut)
	var usage bytes.Buffer
	flags.Usage = func() {
		_, _ = fmt.Fprintln(&usage, "Usage: marklint [options] [inputs]\n       marklint config|rules|sets ACTION [options]")
		flags.SetOutput(&usage)
		flags.PrintDefaults()
		flags.SetOutput(errOut)
	}
	var config, root, format, stdinPath, failOn, explainPath, set, schemaKind, kind string
	var jsonMode, check, fix, fixCheck, showVersion bool
	var network repeated
	flags.StringVar(&config, "config", "", "single-file version-2 configuration")
	flags.StringVar(&root, "root", ".", "lint root")
	flags.StringVar(&format, "format", "text", "text, json, or sarif")
	flags.BoolVar(&jsonMode, "json", false, "emit version-2 JSON envelope")
	flags.StringVar(&stdinPath, "stdin-filepath", "", "root-relative virtual content path")
	flags.StringVar(&failOn, "fail-on", "error", "error, warning, or info")
	flags.StringVar(&explainPath, "path", "", "path to explain")
	flags.StringVar(&set, "set", "", "set to export")
	flags.StringVar(&schemaKind, "schema", "config", "config, report, resolved, check-descriptor, measure-descriptor, or rule-definition")
	flags.StringVar(&kind, "kind", "", "filter checks by document format (markdown or mdx)")
	flags.BoolVar(&check, "check", false, "check canonical formatting")
	flags.BoolVar(&fix, "fix", false, "apply safe edits")
	flags.BoolVar(&fixCheck, "fix-check", false, "preview safe edits")
	flags.BoolVar(&showVersion, "version", false, "print version")
	flags.Var(&network, "allow-provider-network", "permission for an installed remote provider")
	parseErr := flags.Parse(args)
	if errors.Is(parseErr, flag.ErrHelp) {
		if _, err := out.Write(usage.Bytes()); err != nil {
			return 2
		}
		return 0
	}
	provided := map[string]bool{}
	flags.Visit(func(f *flag.Flag) { provided[f.Name] = true })
	if parseErr != nil {
		for i := 0; i < len(args); i++ {
			arg := args[i]
			if arg == "--" {
				break
			}
			if arg == "-" || !strings.HasPrefix(arg, "-") {
				continue
			}
			name, value, assigned := strings.Cut(strings.TrimLeft(arg, "-"), "=")
			option := flags.Lookup(name)
			if option != nil && !assigned {
				boolean, isBool := option.Value.(interface{ IsBoolFlag() bool })
				if (!isBool || !boolean.IsBoolFlag()) && i+1 < len(args) {
					i++
					value, assigned = args[i], true
				}
			}
			if name == "format" && assigned {
				format = value
			}
			if name == "json" && (!assigned || value == "true") {
				jsonMode = true
			}
			if name == "json" && value == "false" {
				jsonMode = false
			}
		}
	}
	if jsonMode {
		if provided["format"] && format != "json" {
			parseErr = fmt.Errorf("--json conflicts with non-JSON format")
		}
		format = "json"
	}
	reportThreshold := failOn
	if !has([]string{"error", "warning", "info"}, reportThreshold) {
		reportThreshold = "error"
	}
	report := NewReport(version, reportThreshold)
	sources := map[string][]byte{}
	emit := func() int {
		report.Finish()
		for _, d := range report.Diagnostics {
			if d.Result == "error" {
				_, _ = fmt.Fprintln(errOut, d.Message)
			}
		}
		var err error
		switch format {
		case "json":
			err = json.NewEncoder(out).Encode(report)
		case "sarif":
			err = json.NewEncoder(out).Encode(sarif(report))
		default:
			for _, d := range report.Diagnostics {
				where := "marklint"
				if d.Location != nil {
					where = fmt.Sprintf("%s:%d:%d", d.Location.Path, d.Location.Start.Line, d.Location.Start.Column)
				}
				_, err = fmt.Fprintf(out, "%s: %s %s [%s] %s\n", where, d.Severity, d.RuleID, d.CheckID, d.Message)
				if err != nil {
					break
				}
				if d.Location != nil {
					if source := sources[d.Location.Path]; len(source) > 0 {
						line := strings.TrimSpace(string(source[lineStart(source, d.Location.StartOffset):lineEnd(source, d.Location.StartOffset)]))
						runes := []rune(line)
						if len(runes) > 180 {
							line = string(runes[:180]) + "…"
						}
						_, err = fmt.Fprintf(out, "  %s\n", line)
					}
				}
				if d.Expected != nil || d.Actual != nil {
					_, err = fmt.Fprintf(out, "  Expected: %v; actual: %v\n", d.Expected, d.Actual)
				}
				for _, related := range d.Related {
					if related.Location != nil {
						_, err = fmt.Fprintf(out, "  %s: %s:%d %s\n", related.Role, related.Location.Path, related.Location.Start.Line, related.Location.Pointer)
					}
				}
				if err != nil {
					break
				}
			}
		}
		if err != nil {
			_, _ = fmt.Fprintln(errOut, err)
			return 2
		}
		return report.ExitCode
	}
	bad := func(code string, err error, loc *Location) int { report.Error(code, err.Error(), loc); return emit() }
	if parseErr != nil {
		return bad("cli.arguments", parseErr, nil)
	}
	if !has([]string{"text", "json", "sarif"}, format) || !has([]string{"error", "warning", "info"}, failOn) {
		return bad("cli.arguments", fmt.Errorf("invalid format %q or failure threshold %q", format, failOn), nil)
	}
	if format == "sarif" && command != "lint" && command != "config validate" {
		return bad("cli.arguments", fmt.Errorf("SARIF output is not supported for %s", command), nil)
	}
	allowed := map[string]string{
		"lint":            "config root format json stdin-filepath fail-on fix fix-check allow-provider-network version",
		"config validate": "config root format json",
		"config schema":   "format json schema",
		"config explain":  "config root format json path",
		"config format":   "config root check",
		"config export":   "config root set",
		"rules list":      "format json kind",
		"rules describe":  "format json",
		"sets list":       "config root format json",
		"sets describe":   "config root format json",
	}
	if options, known := allowed[command]; known {
		for name := range provided {
			if !has(strings.Fields(options), name) {
				return bad("cli.arguments", fmt.Errorf("--%s is not valid for %s", name, command), nil)
			}
		}
		if command != "lint" {
			want := 0
			if command == "rules describe" || command == "sets describe" {
				want = 1
			}
			if len(flags.Args()) != want {
				return bad("cli.arguments", fmt.Errorf("%s requires %d positional arguments", command, want), nil)
			}
		}
	}
	if showVersion {
		if _, err := fmt.Fprintln(out, "marklint", version); err != nil {
			return 2
		}
		return 0
	}
	if config == "-" {
		return bad("cli.arguments", fmt.Errorf("--config - is unsupported; stdin is content"), nil)
	}
	if config == "" {
		candidate := filepath.Join(root, ".marklint.yaml")
		if _, err := os.Stat(candidate); err == nil {
			config = candidate
		} else if !os.IsNotExist(err) {
			return bad("config.read", err, nil)
		}
	}
	if command == "config schema" {
		switch schemaKind {
		case "config":
			return encode(out, ConfigSchemaWithCapabilities(registry, capabilities))
		case "report":
			return encode(out, ReportSchema())
		case "resolved":
			return encode(out, ResolvedRuleSchema(registry))
		case "check-descriptor":
			return encode(out, DescriptorSchema(false))
		case "measure-descriptor":
			return encode(out, DescriptorSchema(true))
		case "rule-definition":
			return encode(out, RuleDefinitionSchema())
		default:
			return bad("cli.arguments", fmt.Errorf("unknown schema kind"), nil)
		}
	}
	if command == "rules list" {
		descriptors := CheckDescriptors(registry)
		if kind != "" {
			if kind != "markdown" && kind != "mdx" {
				return bad("cli.arguments", fmt.Errorf("unsupported catalog kind"), nil)
			}
			filtered := []map[string]any{}
			for _, descriptor := range descriptors {
				if has(list(descriptor["formats"]), kind) {
					filtered = append(filtered, descriptor)
				}
			}
			return encode(out, filtered)
		}
		return encode(out, descriptors)
	}
	if command == "rules describe" {
		if len(flags.Args()) != 1 {
			return bad("cli.arguments", fmt.Errorf("provide a check ID"), nil)
		}
		for _, d := range CheckDescriptors(registry) {
			if d["id"] == flags.Args()[0] {
				return encode(out, d)
			}
		}
		return bad("config.check", fmt.Errorf("unknown check"), nil)
	}
	p := Default(registry)
	p.Capabilities = capabilities
	if config != "" {
		source, err := os.ReadFile(config)
		if err != nil {
			return bad("config.read", err, location(config, nil, 0, 0))
		}
		sources[config] = source
		p, err = DecodeWithCapabilities(source, config, registry, capabilities)
		if err != nil {
			return bad("config.invalid", err, configErrorLocation(config, source, err))
		}
	}
	p.Network = append([]string{}, network...)
	switch command {
	case "config validate":
		if format == "json" || format == "sarif" {
			return emit()
		}
		if _, err := fmt.Fprintln(out, "Configuration is valid."); err != nil {
			return 2
		}
		return 0
	case "config explain":
		if explainPath == "" {
			return bad("cli.arguments", fmt.Errorf("--path is required"), nil)
		}
		if err := Pattern(explainPath); err != nil {
			return bad("cli.arguments", err, nil)
		}
		q, err := p.Policy(explainPath)
		if err != nil {
			return bad("config.routing", err, nil)
		}
		resolved := map[string]Rule{}
		for _, binding := range q.Bindings {
			if rule, ok := p.Rules[binding.Rule]; ok {
				rule.Extends = ""
				resolved[binding.Rule] = rule
			}
		}
		return encode(out, struct {
			Policy
			Rules map[string]Rule `json:"rules"`
		}{q, resolved})
	case "sets list":
		names := sortedKeys(p.bundledSets)
		names = append(names, sortedKeys(p.Config.Sets)...)
		sort.Strings(names)
		return encode(out, names)
	case "sets describe":
		if len(flags.Args()) != 1 {
			return bad("cli.arguments", fmt.Errorf("provide a set ID"), nil)
		}
		ss, bs, ts, err := p.expand(flags.Args())
		if err != nil {
			return bad("config.set", err, nil)
		}
		return encode(out, map[string]any{"sets": ss, "bindings": bs, "templates": ts})
	case "config format":
		formatted, err := p.Format()
		if err != nil {
			return bad("config.format", err, nil)
		}
		if check {
			if bytes.Equal(formatted, p.Source) {
				return 0
			}
			return 1
		}
		if _, err := out.Write(formatted); err != nil {
			return 2
		}
		return 0
	case "config export":
		export, err := p.Export(set, version)
		if err != nil {
			return bad("config.export", err, nil)
		}
		if _, err := out.Write(export); err != nil {
			return 2
		}
		return 0
	case "lint":
	default:
		return bad("cli.arguments", fmt.Errorf("unknown command %s", command), nil)
	}
	paths := flags.Args()
	if len(paths) == 0 {
		paths = []string{"."}
	}
	count := 0
	for _, path := range paths {
		if path == "-" {
			count++
		}
	}
	if count > 1 || stdinPath != "" && count == 0 {
		return bad("cli.arguments", fmt.Errorf("one stdin marker is allowed; --stdin-filepath requires -"), nil)
	}
	if count > 0 && (fix || fixCheck) {
		return bad("cli.arguments", fmt.Errorf("fix modes cannot be combined with stdin"), nil)
	}
	if fix && fixCheck {
		return bad("cli.arguments", fmt.Errorf("fix modes conflict"), nil)
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return bad("input.root", err, nil)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return bad("input.root", err, nil)
	}
	files, err := Discover(root, paths, p.Config.Exclude)
	if err != nil {
		return bad("input.read", err, nil)
	}
	docs := []*Document{}
	parse := func(path string, source []byte, kind string) error {
		sources[path] = source
		if !utf8.Valid(source) {
			return fmt.Errorf("input is not valid UTF-8")
		}
		var d *Document
		var err error
		if strings.EqualFold(filepath.Ext(path), ".mdx") {
			d, err = ParseMDX(ctx, path, source, kind, p.Config.Components)
		} else {
			d = Parse(path, source, kind)
		}
		if err != nil {
			return err
		}
		docs = append(docs, d)
		return nil
	}
	identities := map[string]bool{}
	for _, file := range files {
		rel, _ := filepath.Rel(root, file)
		rel = filepath.ToSlash(rel)
		identities[inputIdentity(rel)] = true
		source, err := os.ReadFile(file)
		if err != nil {
			return bad("input.read", err, location(rel, nil, 0, 0))
		}
		if err := parse(rel, source, "file"); err != nil {
			return bad("input.parse", err, location(rel, source, 0, 0))
		}
	}
	if count > 0 {
		if stdinPath == "" {
			stdinPath = "stdin.md"
		}
		if err := Pattern(stdinPath); err != nil {
			return bad("input.path", err, nil)
		}
		if strings.ContainsAny(stdinPath, "*?[") || !supported(stdinPath) {
			return bad("input.path", fmt.Errorf("stdin requires a literal supported root-relative filename"), nil)
		}
		if identities[inputIdentity(stdinPath)] {
			return bad("input.identity", fmt.Errorf("stdin identity collides with a disk input"), nil)
		}
		if !Matches(p.Config.Exclude, stdinPath) {
			source, err := io.ReadAll(in)
			if err != nil {
				return bad("input.read", err, nil)
			}
			if err := parse(stdinPath, source, "stdin"); err != nil {
				return bad("input.parse", err, location(stdinPath, source, 0, 0))
			}
		}
	}
	if len(docs) == 0 {
		return bad("input.empty", fmt.Errorf("no selected inputs"), nil)
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].Path < docs[j].Path })
	p.Run(ctx, root, docs, report)
	if (fix || fixCheck) && report.ExitCode != 2 {
		for _, finding := range p.Fixes {
			if err := interfaces.CheckPathRoot(root, finding.Path); err != nil {
				return bad("fix.path", err, nil)
			}
		}
		var plan engine.FixPlan
		var err error
		if fix {
			plan, err = engine.ApplyFixes(ctx, p.Fixes)
		} else {
			plan, err = engine.DryRunFixes(ctx, p.Fixes)
		}
		if err != nil {
			return bad("fix.execution", err, nil)
		}
		if fix && plan.HasAcceptedEdits() {
			docs = nil
			for _, file := range files {
				rel, _ := filepath.Rel(root, file)
				source, err := os.ReadFile(file)
				if err != nil {
					return bad("input.read", err, nil)
				}
				if err := parse(filepath.ToSlash(rel), source, "file"); err != nil {
					return bad("input.parse", err, nil)
				}
			}
			report = NewReport(version, failOn)
			p.Run(ctx, root, docs, report)
		}
		if fixCheck && format == "text" {
			_, _ = fmt.Fprintf(errOut, "%d safe edits available; %d suggestions rejected.\n", len(plan.Accepted), len(plan.Rejected))
		}
	}
	return emit()
}

func inputIdentity(path string) string {
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}

func configErrorLocation(path string, source []byte, err error) *Location {
	p := &Program{Path: path, Source: source}
	if yaml.Unmarshal(source, &p.tree) != nil {
		return location(path, source, 0, 0)
	}
	var validation *jsonschema.ValidationError
	if errors.As(err, &validation) {
		pointer := ""
		var visit func(*jsonschema.ValidationError)
		visit = func(v *jsonschema.ValidationError) {
			candidate := "/" + strings.Join(v.InstanceLocation, "/")
			if len(candidate) > len(pointer) {
				pointer = candidate
			}
			for _, cause := range v.Causes {
				visit(cause)
			}
		}
		visit(validation)
		return p.origin(pointer, 0)
	}
	message := err.Error()
	for _, category := range []string{"rule", "template"} {
		prefix := category + " "
		if strings.HasPrefix(message, prefix) {
			name, detail, _ := strings.Cut(strings.TrimPrefix(message, prefix), ":")
			line := 0
			if category == "template" {
				_, _ = fmt.Sscanf(strings.TrimSpace(detail), "line %d:", &line)
			}
			return p.origin("/"+category+"s/"+name, line)
		}
	}
	return location(path, source, 0, 0)
}
func encode(out io.Writer, value any) int {
	if err := json.NewEncoder(out).Encode(value); err != nil {
		return 2
	}
	return 0
}
func supported(path string) bool {
	return has([]string{".md", ".markdown", ".mdx"}, strings.ToLower(filepath.Ext(path)))
}
func Discover(root string, paths, exclude []string) ([]string, error) {
	seen := map[string]bool{}
	files := []string{}
	add := func(file string) error {
		real, err := filepath.EvalSymlinks(file)
		if err != nil {
			return err
		}
		real, err = filepath.Abs(real)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, real)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("input %s escapes document root", file)
		}
		if Matches(exclude, filepath.ToSlash(rel)) {
			return nil
		}
		if !seen[real] {
			seen[real] = true
			files = append(files, real)
		}
		return nil
	}
	for _, input := range paths {
		if input == "-" {
			continue
		}
		info, err := os.Stat(input)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			if !supported(input) {
				return nil, fmt.Errorf("unsupported file %s", input)
			}
			if err := add(input); err != nil {
				return nil, err
			}
			continue
		}
		if err := interfaces.CheckPathRoot(root, input); err != nil {
			return nil, err
		}
		if info, err := os.Lstat(input); err != nil {
			return nil, err
		} else if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("directory symlinks are not followed")
		}
		err = filepath.WalkDir(input, func(file string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if entry.Name() == ".git" || entry.Name() == "node_modules" {
					return filepath.SkipDir
				}
				absolute, err := filepath.Abs(file)
				if err != nil {
					return err
				}
				rel, err := filepath.Rel(root, absolute)
				if err != nil {
					return err
				}
				if Matches(exclude, filepath.ToSlash(rel)) {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if supported(file) {
				return add(file)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(files)
	return files, nil
}
func (p *Program) Format() ([]byte, error) {
	var out bytes.Buffer
	e := yaml.NewEncoder(&out)
	e.SetIndent(2)
	tree := p.tree
	order := strings.Fields("version language profile components rules templates sets apply exclude suppressions")
	n := tree.Content[0]
	byKey := map[string][2]*yaml.Node{}
	for i := 0; i < len(n.Content); i += 2 {
		byKey[n.Content[i].Value] = [2]*yaml.Node{n.Content[i], n.Content[i+1]}
	}
	copyNode := *n
	copyNode.Content = nil
	for _, k := range order {
		if pair, ok := byKey[k]; ok {
			copyNode.Content = append(copyNode.Content, pair[0], pair[1])
		}
	}
	tree.Content = []*yaml.Node{&copyNode}
	var literalize func(*yaml.Node)
	literalize = func(n *yaml.Node) {
		if n.Kind == yaml.ScalarNode && n.Tag == "!!str" && strings.Contains(n.Value, "\n") {
			n.Style = yaml.LiteralStyle
		}
		for _, c := range n.Content {
			literalize(c)
		}
	}
	literalize(&tree)
	if err := e.Encode(&tree); err != nil {
		return nil, err
	}
	if err := e.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
func (p *Program) Export(set, version string) ([]byte, error) {
	if set == "" {
		return nil, fmt.Errorf("--set is required")
	}
	ss, bs, ts, err := p.expand([]string{set})
	if err != nil {
		return nil, err
	}
	c := Config{Version: 2, Language: p.Config.Language, Profile: p.Config.Profile, Components: p.Config.Components, Rules: map[string]Rule{}, Sets: map[string]Set{}, Templates: map[string]string{}, Apply: []Apply{{Use: []string{set}}}}
	var addRule func(string)
	addRule = func(id string) {
		if _, ok := c.Rules[id]; ok {
			return
		}
		r, ok := p.Config.Rules[id]
		if !ok {
			return
		}
		c.Rules[id] = r
		if r.Extends != "" {
			addRule(r.Extends)
		}
		if r.Check == "markdown.table-schema" {
			for _, col := range object(r.Options["columns"]) {
				for _, ref := range list(object(col)["rules"]) {
					addRule(ref)
				}
			}
		}
	}
	for _, s := range ss {
		if v, ok := p.Config.Sets[s]; ok {
			c.Sets[s] = v
		}
	}
	for _, b := range bs {
		addRule(b.Rule)
	}
	for _, name := range ts {
		c.Templates[name] = p.Config.Templates[name]
		for _, r := range p.Templates[name].Requirements {
			for _, id := range r.IDs {
				if id != "" {
					addRule(id)
				}
			}
		}
	}
	for _, s := range p.Config.Suppressions {
		if _, ok := c.Rules[s.Rule]; ok {
			c.Suppressions = append(c.Suppressions, s)
		} else {
			for _, name := range ts {
				if strings.HasPrefix(s.Rule, "template."+name+".") {
					c.Suppressions = append(c.Suppressions, s)
					break
				}
			}
		}
	}
	data, err := yaml.Marshal(c)
	return append([]byte("# Exported by marklint "+version+"; routing replaced with catch-all set "+set+".\n"), data...), err
}
func sarif(r *Report) map[string]any {
	results := []any{}
	for _, d := range r.Diagnostics {
		level := "note"
		switch d.Severity {
		case "error":
			level = "error"
		case "warning":
			level = "warning"
		}
		result := map[string]any{"ruleId": d.RuleID, "level": level, "message": map[string]string{"text": d.Message}}
		if d.Location != nil {
			result["locations"] = []any{map[string]any{"physicalLocation": map[string]any{"artifactLocation": map[string]string{"uri": d.Location.Path}, "region": map[string]int{"startLine": d.Location.Start.Line, "startColumn": d.Location.Start.Column}}}}
		}
		results = append(results, result)
	}
	return map[string]any{"version": "2.1.0", "$schema": "https://json.schemastore.org/sarif-2.1.0.json", "runs": []any{map[string]any{"tool": map[string]any{"driver": map[string]string{"name": "marklint", "version": r.Tool["version"]}}, "results": results}}}
}
