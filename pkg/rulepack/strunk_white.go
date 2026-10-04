package rulepack

type strunkSpec struct {
	name, pattern, guidance string
}

// These finite lexical signals invite editorial review. They are not a grammar parser.
var strunkSpecs = []strunkSpec{
	{"needless-words", `(?i)\b(?:in\s+order\s+to|due\s+to\s+the\s+fact\s+that|at\s+this\s+point\s+in\s+time|in\s+the\s+event\s+that|for\s+the\s+purpose\s+of|the\s+question\s+as\s+to\s+whether|in\s+a\s+manner\s+that|on\s+a\s+daily\s+basis)\b`, "consider a shorter expression while preserving meaning"},
	{"fancy-words", `(?i)\b(?:utilize|utilization|aforementioned|heretofore|henceforth)\b`, "consider a familiar word such as use, earlier, or from now on"},
	{"qualifiers", `(?i)\b(?:very|really|rather|quite)\b`, "review this qualifier; a precise description may be stronger"},
	{"negative-phrases", `(?i)\b(?:not\s+uncommon|not\s+impossible|not\s+unlikely|not\s+without)\b`, "consider a positive statement if the nuance permits it"},
	{"passive-voice", `(?i)\b(?:is|are|was|were|be|been|being)\s+(?:(?:carefully|automatically|explicitly)\s+)?(?:approved|created|deleted|modified|sent|written|read|processed|returned|requested|performed|used|made|done|seen|given|taken|built|stored|generated)\s+by\b`, "possible agent-bearing passive; consider naming the actor first"},
	{"usage", `(?i)\b(?:irregardless|(?:could|should|would|might|must)\s+of)\b`, "review conventional usage: regardless, or a modal followed by have"},
	{"exclamations", `!+`, "review emphatic punctuation; factual prose usually needs a period"},
	{"existential-openings", `(?im)(?:^|[.!?]\s+)(there\s+(?:is|are|was|were))\b`, "consider opening with the subject instead of there"},
	{"redundant-pairs", `(?i)\b(?:advance\s+planning|basic\s+fundamentals|final\s+outcome|each\s+and\s+every|free\s+gift|past\s+history)\b`, "review this pair for duplicated meaning"},
	{"correlative-pairs", `(?i)\b(?:both\b[^.!?;]*?\bor|either\b[^.!?;]*?\band|neither\b[^.!?;]*?\bor)\b`, "review the paired construction: both/and, either/or, or neither/nor"},
}

// Strunk and White checks are named default configurations of the generic matcher.
func registerStrunkWhite(r *Registry) error {
	for _, spec := range strunkSpecs {
		id := "text.strunk-white." + spec.name
		if err := r.Register(id, strunkFactory(id, spec)); err != nil {
			return err
		}
	}
	return nil
}
func strunkDefaults(spec strunkSpec) matcherOptions {
	options := matcherDefaults()
	options.Patterns = []string{spec.pattern}
	options.Message = spec.guidance
	if spec.name == "existential-openings" {
		options.CaptureGroup = 1
	}
	if spec.name == "correlative-pairs" {
		options.PairedConjunctions = map[string]string{"both": "and", "either": "or", "neither": "nor"}
	}
	return options
}
func strunkFactory(id string, spec strunkSpec) Factory {
	return matcherFactory(id, strunkDefaults(spec))
}
