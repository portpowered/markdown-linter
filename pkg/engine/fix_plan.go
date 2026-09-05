package engine

import (
	"context"
	"fmt"
	"os"
	"sort"
)

// FixRejectionReason identifies why a suggested fix could not be accepted.
type FixRejectionReason string

const (
	// FixRejectionNoSafeFix means the diagnostic had no fix matching the planner confidence.
	FixRejectionNoSafeFix FixRejectionReason = "no-safe-fix"
	// FixRejectionInvalidRange means at least one edit addressed an invalid source range.
	FixRejectionInvalidRange FixRejectionReason = "invalid-range"
	// FixRejectionOverlappingEdit means at least one edit overlapped an accepted edit.
	FixRejectionOverlappingEdit FixRejectionReason = "overlapping-edit"
)

// FixPlanOption configures fix planning and application.
type FixPlanOption func(*fixPlanConfig)

type fixPlanConfig struct {
	confidence FixConfidence
}

// WithFixConfidence configures the confidence required for automatic fix application.
func WithFixConfidence(confidence FixConfidence) FixPlanOption {
	return func(config *fixPlanConfig) {
		config.confidence = confidence
	}
}

// PlannedEdit is one accepted text edit selected from a diagnostic suggested fix.
type PlannedEdit struct {
	Path       string
	RuleID     string
	FixTitle   string
	TextEdit   TextEdit
	Diagnostic Diagnostic
}

// RejectedFix records a diagnostic fix that was intentionally not accepted.
type RejectedFix struct {
	Path       string
	RuleID     string
	FixTitle   string
	Reason     FixRejectionReason
	Message    string
	Diagnostic Diagnostic
}

// FileFixPlan groups accepted edits for one file.
type FileFixPlan struct {
	Path  string
	Edits []PlannedEdit
}

// FixPlan describes accepted and rejected fixes for a diagnostic set.
type FixPlan struct {
	Files    []FileFixPlan
	Accepted []PlannedEdit
	Rejected []RejectedFix
}

// FixReviewReport groups a fix plan and diagnostics by contributor review outcome.
type FixReviewReport struct {
	Accepted        []PlannedEdit
	Rejected        []RejectedFix
	Ambiguous       []Diagnostic
	OtherNonFixable []Diagnostic
}

// HasAcceptedEdits reports whether the plan contains any writeable edits.
func (p FixPlan) HasAcceptedEdits() bool {
	return len(p.Accepted) > 0
}

// NewFixReviewReport classifies fix output into accepted, rejected, ambiguous, and other manual-review outcomes.
func NewFixReviewReport(plan FixPlan, diagnostics []Diagnostic) FixReviewReport {
	report := FixReviewReport{
		Accepted: append([]PlannedEdit(nil), plan.Accepted...),
		Rejected: append([]RejectedFix(nil), plan.Rejected...),
	}

	accepted := acceptedDiagnosticKeys(plan.Accepted)
	rejected := rejectedDiagnosticKeys(plan.Rejected)
	for _, diagnostic := range diagnostics {
		key := newDiagnosticKey(diagnostic)
		if _, ok := accepted[key]; ok {
			continue
		}
		if _, ok := rejected[key]; ok {
			continue
		}
		if len(diagnostic.SuggestedFixes) > 0 {
			continue
		}

		if diagnostic.Category == DiagnosticCategoryRelocationAmbiguous {
			report.Ambiguous = append(report.Ambiguous, diagnostic)
			continue
		}
		report.OtherNonFixable = append(report.OtherNonFixable, diagnostic)
	}

	sortDiagnostics(report.Ambiguous)
	sortDiagnostics(report.OtherNonFixable)
	return report
}

// PlanFixes selects non-overlapping safe fixes from diagnostics without changing files.
func PlanFixes(diagnostics []Diagnostic, sources map[string][]byte, options ...FixPlanOption) FixPlan {
	config := fixPlanConfig{confidence: FixConfidenceSafe}
	for _, option := range options {
		option(&config)
	}

	candidates := buildFixCandidates(diagnostics, config.confidence)
	sort.SliceStable(candidates, func(i, j int) bool {
		return compareFixCandidates(candidates[i], candidates[j])
	})

	acceptedByPath := map[string][]PlannedEdit{}
	var accepted []PlannedEdit
	var rejected []RejectedFix

	for _, candidate := range candidates {
		source, ok := sources[candidate.diagnostic.Path]
		if !ok {
			rejected = append(rejected, candidate.rejection(FixRejectionInvalidRange, "source content is unavailable for fix planning"))
			continue
		}
		if invalid, message := firstInvalidRange(candidate.edits, len(source)); invalid {
			rejected = append(rejected, candidate.rejection(FixRejectionInvalidRange, message))
			continue
		}
		if overlapsAccepted(candidate.edits, acceptedByPath[candidate.diagnostic.Path]) {
			rejected = append(rejected, candidate.rejection(FixRejectionOverlappingEdit, "suggested fix overlaps an already accepted edit"))
			continue
		}

		for _, edit := range candidate.edits {
			planned := PlannedEdit{
				Path:       candidate.diagnostic.Path,
				RuleID:     candidate.diagnostic.RuleID,
				FixTitle:   candidate.fix.Title,
				TextEdit:   edit,
				Diagnostic: candidate.diagnostic,
			}
			accepted = append(accepted, planned)
			acceptedByPath[planned.Path] = append(acceptedByPath[planned.Path], planned)
		}
	}

	for _, diagnostic := range diagnostics {
		if len(diagnostic.SuggestedFixes) == 0 {
			continue
		}
		if hasFixWithConfidence(diagnostic.SuggestedFixes, config.confidence) {
			continue
		}
		rejected = append(rejected, RejectedFix{
			Path:       diagnostic.Path,
			RuleID:     diagnostic.RuleID,
			Reason:     FixRejectionNoSafeFix,
			Message:    "diagnostic has no suggested fix with required confidence",
			Diagnostic: diagnostic,
		})
	}

	sort.SliceStable(accepted, func(i, j int) bool {
		return comparePlannedEdits(accepted[i], accepted[j])
	})
	sort.SliceStable(rejected, func(i, j int) bool {
		return compareRejectedFixes(rejected[i], rejected[j])
	})

	return FixPlan{
		Files:    buildFileFixPlans(accepted),
		Accepted: accepted,
		Rejected: rejected,
	}
}

// DryRunFixes plans fixes against the current file contents without writing changes.
func DryRunFixes(ctx context.Context, diagnostics []Diagnostic, options ...FixPlanOption) (FixPlan, error) {
	sources, err := readDiagnosticSources(ctx, diagnostics)
	if err != nil {
		return FixPlan{}, err
	}
	return PlanFixes(diagnostics, sources, options...), nil
}

// ApplyFixes plans fixes against the current file contents and writes files with accepted edits.
func ApplyFixes(ctx context.Context, diagnostics []Diagnostic, options ...FixPlanOption) (FixPlan, error) {
	sources, err := readDiagnosticSources(ctx, diagnostics)
	if err != nil {
		return FixPlan{}, err
	}

	plan := PlanFixes(diagnostics, sources, options...)
	for _, file := range plan.Files {
		if err := ctx.Err(); err != nil {
			return FixPlan{}, err
		}
		source := sources[file.Path]
		updated := applyPlannedEdits(source, file.Edits)
		if string(updated) == string(source) {
			continue
		}
		mode, err := fileMode(file.Path)
		if err != nil {
			return FixPlan{}, err
		}
		if err := os.WriteFile(file.Path, updated, mode); err != nil {
			return FixPlan{}, fmt.Errorf("writing markdown fixes to %q: %w", file.Path, err)
		}
	}

	return plan, nil
}

type fixCandidate struct {
	diagnostic Diagnostic
	fix        SuggestedFix
	edits      []TextEdit
}

func buildFixCandidates(diagnostics []Diagnostic, confidence FixConfidence) []fixCandidate {
	var candidates []fixCandidate
	for _, diagnostic := range diagnostics {
		for _, fix := range diagnostic.SuggestedFixes {
			if fix.Confidence != confidence {
				continue
			}
			candidates = append(candidates, fixCandidate{
				diagnostic: diagnostic,
				fix:        fix,
				edits:      append([]TextEdit(nil), fix.Edits...),
			})
			break
		}
	}
	return candidates
}

func (c fixCandidate) rejection(reason FixRejectionReason, message string) RejectedFix {
	return RejectedFix{
		Path:       c.diagnostic.Path,
		RuleID:     c.diagnostic.RuleID,
		FixTitle:   c.fix.Title,
		Reason:     reason,
		Message:    message,
		Diagnostic: c.diagnostic,
	}
}

func firstInvalidRange(edits []TextEdit, sourceLength int) (bool, string) {
	if len(edits) == 0 {
		return true, "suggested fix has no edits"
	}
	for _, edit := range edits {
		if edit.StartOffset < 0 || edit.EndOffset < edit.StartOffset || edit.EndOffset > sourceLength {
			return true, fmt.Sprintf("invalid edit range [%d,%d) for source length %d", edit.StartOffset, edit.EndOffset, sourceLength)
		}
	}
	return false, ""
}

func overlapsAccepted(edits []TextEdit, accepted []PlannedEdit) bool {
	for _, edit := range edits {
		for _, planned := range accepted {
			if rangesOverlap(edit.StartOffset, edit.EndOffset, planned.TextEdit.StartOffset, planned.TextEdit.EndOffset) {
				return true
			}
		}
	}
	return rangesOverlapEachOther(edits)
}

func rangesOverlapEachOther(edits []TextEdit) bool {
	sorted := append([]TextEdit(nil), edits...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].StartOffset != sorted[j].StartOffset {
			return sorted[i].StartOffset < sorted[j].StartOffset
		}
		return sorted[i].EndOffset < sorted[j].EndOffset
	})
	for index := 1; index < len(sorted); index++ {
		if rangesOverlap(sorted[index-1].StartOffset, sorted[index-1].EndOffset, sorted[index].StartOffset, sorted[index].EndOffset) {
			return true
		}
	}
	return false
}

func rangesOverlap(firstStart int, firstEnd int, secondStart int, secondEnd int) bool {
	if firstStart == firstEnd && secondStart == secondEnd && firstStart == secondStart {
		return true
	}
	return firstStart < secondEnd && secondStart < firstEnd
}

func hasFixWithConfidence(fixes []SuggestedFix, confidence FixConfidence) bool {
	for _, fix := range fixes {
		if fix.Confidence == confidence {
			return true
		}
	}
	return false
}

func buildFileFixPlans(edits []PlannedEdit) []FileFixPlan {
	byPath := map[string][]PlannedEdit{}
	for _, edit := range edits {
		byPath[edit.Path] = append(byPath[edit.Path], edit)
	}

	paths := make([]string, 0, len(byPath))
	for path := range byPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	files := make([]FileFixPlan, 0, len(paths))
	for _, path := range paths {
		fileEdits := byPath[path]
		sort.SliceStable(fileEdits, func(i, j int) bool {
			if fileEdits[i].TextEdit.StartOffset != fileEdits[j].TextEdit.StartOffset {
				return fileEdits[i].TextEdit.StartOffset < fileEdits[j].TextEdit.StartOffset
			}
			return fileEdits[i].TextEdit.EndOffset < fileEdits[j].TextEdit.EndOffset
		})
		files = append(files, FileFixPlan{Path: path, Edits: fileEdits})
	}
	return files
}

func applyPlannedEdits(source []byte, edits []PlannedEdit) []byte {
	sorted := append([]PlannedEdit(nil), edits...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].TextEdit.StartOffset > sorted[j].TextEdit.StartOffset
	})

	updated := append([]byte(nil), source...)
	for _, planned := range sorted {
		edit := planned.TextEdit
		replacement := []byte(edit.Replacement)
		next := make([]byte, 0, len(updated)-(edit.EndOffset-edit.StartOffset)+len(replacement))
		next = append(next, updated[:edit.StartOffset]...)
		next = append(next, replacement...)
		next = append(next, updated[edit.EndOffset:]...)
		updated = next
	}
	return updated
}

func readDiagnosticSources(ctx context.Context, diagnostics []Diagnostic) (map[string][]byte, error) {
	paths := map[string]struct{}{}
	for _, diagnostic := range diagnostics {
		if len(diagnostic.SuggestedFixes) == 0 {
			continue
		}
		paths[diagnostic.Path] = struct{}{}
	}

	sources := make(map[string][]byte, len(paths))
	for path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading markdown fixes source %q: %w", path, err)
		}
		sources[path] = source
	}
	return sources, nil
}

func fileMode(path string) (os.FileMode, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("stat markdown fixes target %q: %w", path, err)
	}
	return info.Mode().Perm(), nil
}

func compareFixCandidates(first fixCandidate, second fixCandidate) bool {
	if first.diagnostic.Path != second.diagnostic.Path {
		return first.diagnostic.Path < second.diagnostic.Path
	}
	firstStart, firstEnd := candidateRange(first)
	secondStart, secondEnd := candidateRange(second)
	if firstStart != secondStart {
		return firstStart < secondStart
	}
	if firstEnd != secondEnd {
		return firstEnd < secondEnd
	}
	if first.diagnostic.RuleID != second.diagnostic.RuleID {
		return first.diagnostic.RuleID < second.diagnostic.RuleID
	}
	return first.fix.Title < second.fix.Title
}

func candidateRange(candidate fixCandidate) (int, int) {
	if len(candidate.edits) == 0 {
		return candidate.diagnostic.StartOffset, candidate.diagnostic.EndOffset
	}
	start := candidate.edits[0].StartOffset
	end := candidate.edits[0].EndOffset
	for _, edit := range candidate.edits[1:] {
		if edit.StartOffset < start {
			start = edit.StartOffset
		}
		if edit.EndOffset > end {
			end = edit.EndOffset
		}
	}
	return start, end
}

func comparePlannedEdits(first PlannedEdit, second PlannedEdit) bool {
	if first.Path != second.Path {
		return first.Path < second.Path
	}
	if first.TextEdit.StartOffset != second.TextEdit.StartOffset {
		return first.TextEdit.StartOffset < second.TextEdit.StartOffset
	}
	if first.TextEdit.EndOffset != second.TextEdit.EndOffset {
		return first.TextEdit.EndOffset < second.TextEdit.EndOffset
	}
	if first.RuleID != second.RuleID {
		return first.RuleID < second.RuleID
	}
	return first.FixTitle < second.FixTitle
}

func compareRejectedFixes(first RejectedFix, second RejectedFix) bool {
	if first.Path != second.Path {
		return first.Path < second.Path
	}
	if first.Diagnostic.StartOffset != second.Diagnostic.StartOffset {
		return first.Diagnostic.StartOffset < second.Diagnostic.StartOffset
	}
	if first.RuleID != second.RuleID {
		return first.RuleID < second.RuleID
	}
	return first.FixTitle < second.FixTitle
}

type diagnosticKey struct {
	path        string
	line        int
	startOffset int
	endOffset   int
	ruleID      string
	message     string
}

func acceptedDiagnosticKeys(accepted []PlannedEdit) map[diagnosticKey]struct{} {
	keys := map[diagnosticKey]struct{}{}
	for _, edit := range accepted {
		keys[newDiagnosticKey(edit.Diagnostic)] = struct{}{}
	}
	return keys
}

func rejectedDiagnosticKeys(rejected []RejectedFix) map[diagnosticKey]struct{} {
	keys := map[diagnosticKey]struct{}{}
	for _, fix := range rejected {
		keys[newDiagnosticKey(fix.Diagnostic)] = struct{}{}
	}
	return keys
}

func newDiagnosticKey(diagnostic Diagnostic) diagnosticKey {
	return diagnosticKey{
		path:        diagnostic.Path,
		line:        diagnostic.Line,
		startOffset: diagnostic.StartOffset,
		endOffset:   diagnostic.EndOffset,
		ruleID:      diagnostic.RuleID,
		message:     diagnostic.Message,
	}
}

func sortDiagnostics(diagnostics []Diagnostic) {
	sort.SliceStable(diagnostics, func(i, j int) bool {
		if diagnostics[i].Path != diagnostics[j].Path {
			return diagnostics[i].Path < diagnostics[j].Path
		}
		if diagnostics[i].Line != diagnostics[j].Line {
			return diagnostics[i].Line < diagnostics[j].Line
		}
		if diagnostics[i].RuleID != diagnostics[j].RuleID {
			return diagnostics[i].RuleID < diagnostics[j].RuleID
		}
		return diagnostics[i].Message < diagnostics[j].Message
	})
}
