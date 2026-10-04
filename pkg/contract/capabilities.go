package contract

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// Descriptor is installation metadata, never evaluator code loaded from YAML.
type Descriptor struct {
	ID                  string         `json:"id"`
	Version             string         `json:"version"`
	Summary             string         `json:"summary"`
	Targets             []string       `json:"targets"`
	Formats             []string       `json:"formats"`
	Languages           []string       `json:"languages"`
	Capabilities        []string       `json:"capabilities"`
	ParametersSchema    map[string]any `json:"parametersSchema"`
	Execution           string         `json:"execution"`
	Fixtures            []any          `json:"fixtures"`
	Unit                string         `json:"unit"`
	ValueType           string         `json:"valueType"`
	Granularity         string         `json:"granularity"`
	Views               []string       `json:"views"`
	Range               *[2]float64    `json:"range,omitempty"`
	SentenceAttribution bool           `json:"sentenceAttribution,omitempty"`
}
type MeasureInput struct {
	Path, Text, Language, Profile, Kind, Scope, View string
	StartOffset, EndOffset                           int
	Parameters                                       map[string]any
	TargetText                                       string
	TargetStartOffset, TargetEndOffset               int
}
type Sample struct {
	Status                 string
	Value                  float64
	StartOffset, EndOffset int
	Text                   string
	Inputs                 map[string]any
	Classification         string
}
type MeasureFunc func(context.Context, MeasureInput) ([]Sample, error)
type InstalledMeasure struct {
	Descriptor Descriptor
	Evaluate   MeasureFunc
}

// Capabilities registers trusted local measures and provider profiles at build time.
// A provider must publish its native output identity, scale and attribution contract.
type Capabilities struct {
	Measures  map[string]InstalledMeasure
	Providers map[string]map[string]InstalledMeasure
}

func NewCapabilities() *Capabilities {
	return &Capabilities{Measures: map[string]InstalledMeasure{}, Providers: map[string]map[string]InstalledMeasure{}}
}
func validateDescriptor(d Descriptor, fn MeasureFunc) error {
	if !nameRE.MatchString(d.ID) || d.Version == "" || d.Summary == "" || fn == nil || len(d.Targets) == 0 || len(d.Formats) == 0 || len(d.Languages) == 0 {
		return fmt.Errorf("measure identity, version, summary, targets, formats, languages and evaluator required")
	}
	if !has([]string{"local", "remote"}, d.Execution) || !has([]string{"integer", "number"}, d.ValueType) || !has([]string{"target", "window"}, d.Granularity) || d.Unit == "" || len(d.Views) == 0 || d.ParametersSchema == nil {
		return fmt.Errorf("invalid measure descriptor")
	}
	if d.Range != nil && (math.IsNaN(d.Range[0]) || math.IsNaN(d.Range[1]) || math.IsInf(d.Range[0], 0) || math.IsInf(d.Range[1], 0) || d.Range[0] > d.Range[1]) {
		return fmt.Errorf("invalid scale")
	}
	return nil
}
func (c *Capabilities) RegisterMeasure(d Descriptor, fn MeasureFunc) error {
	if err := validateDescriptor(d, fn); err != nil {
		return err
	}
	if _, ok := measures[d.ID]; ok {
		return fmt.Errorf("reserved measure %s", d.ID)
	}
	if _, ok := c.Measures[d.ID]; ok {
		return fmt.Errorf("duplicate measure %s", d.ID)
	}
	c.Measures[d.ID] = InstalledMeasure{d, fn}
	return nil
}
func (c *Capabilities) RegisterProvider(id string, d Descriptor, fn MeasureFunc) error {
	if !nameRE.MatchString(id) {
		return fmt.Errorf("invalid provider ID")
	}
	if err := validateDescriptor(d, fn); err != nil {
		return err
	}
	if c.Providers[id] == nil {
		c.Providers[id] = map[string]InstalledMeasure{}
	}
	if _, ok := c.Providers[id][d.ID]; ok {
		return fmt.Errorf("duplicate provider output")
	}
	c.Providers[id][d.ID] = InstalledMeasure{d, fn}
	return nil
}
func (c *Capabilities) lookup(r Rule) (InstalledMeasure, bool) {
	m := str(r.Options, "measure", "")
	if m == "provider.score" {
		params := object(r.Options["parameters"])
		v, ok := c.Providers[str(params, "provider", "")][str(params, "measure", "")]
		return v, ok
	}
	v, ok := c.Measures[m]
	return v, ok
}
func (e *execution) installedMeasure(id string, r Rule, t *Target, scope string, related []Related, installed InstalledMeasure) {
	d := installed.Descriptor
	language := e.document.Policy.Language
	if !has(d.Formats, e.document.Format) || !has(d.Languages, "*") && !has(d.Languages, language) {
		e.report.Error("capability.measure", "Installed measure does not support this format/language.", location(e.document.Path, e.document.Source, t.Start, t.End))
		return
	}
	params := object(r.Options["parameters"])
	provider := str(params, "provider", "")
	if d.Execution == "remote" && !has(e.program.Network, provider) {
		e.report.Error("capability.network", "Remote provider requires --allow-provider-network "+provider, e.program.origin("/rules/"+id, 0))
		return
	}
	units := e.document.units(t, str(r.Options, "view", "auto"), scope)
	texts := []string{}
	for _, u := range units {
		texts = append(texts, u.Text)
	}
	input := MeasureInput{Path: e.document.Path, Text: strings.Join(texts, "\n\n"), Language: language, Profile: e.document.Policy.Profile, Kind: t.Kind, Scope: scope, View: str(r.Options, "view", "auto"), StartOffset: t.Start, EndOffset: t.End, Parameters: params}
	paragraphContext := str(params, "context", "target") == "paragraph"
	if paragraphContext {
		if t.Kind != "sentence" || t.Parent == nil || t.Parent.Kind != "paragraph" || !d.SentenceAttribution {
			e.report.Error("capability.context", "Paragraph context requires native sentence attribution.", location(e.document.Path, e.document.Source, t.Start, t.End))
			return
		}
		input.TargetText, input.TargetStartOffset, input.TargetEndOffset = input.Text, t.Start, t.End
		input.Text = t.Parent.Units[0].Text
		input.StartOffset, input.EndOffset = t.Parent.Start, t.Parent.End
	}
	encoded, _ := json.Marshal(input)
	key := provider + "|" + d.ID + "|" + d.Version + "|" + string(encoded)
	if e.samples == nil {
		e.samples = map[string][]Sample{}
	}
	samples, cached := e.samples[key]
	if !cached {
		var err error
		samples, err = installed.Evaluate(e.ctx, input)
		if err != nil {
			e.report.Error("execution.measure", err.Error(), location(e.document.Path, e.document.Source, t.Start, t.End))
			return
		}
		e.samples[key] = samples
	}
	if len(samples) == 0 {
		e.report.Error("execution.measure", "Measure returned no samples.", location(e.document.Path, e.document.Source, t.Start, t.End))
		return
	}
	if d.Granularity == "target" && len(samples) != 1 {
		e.report.Error("execution.measure", "Target measure returned multiple samples.", nil)
		return
	}
	for _, sample := range samples {
		if sample.Status == "unknown" {
			e.unknown(id, r, t, related, "measure.unknown", "Installed measure could not establish a known result.")
			continue
		}
		if sample.Status != "known" || math.IsNaN(sample.Value) || math.IsInf(sample.Value, 0) || d.ValueType == "integer" && (sample.Value < 0 || sample.Value != math.Trunc(sample.Value)) || d.Range != nil && (sample.Value < d.Range[0] || sample.Value > d.Range[1]) {
			e.report.Error("execution.measure", "Invalid sample or value outside the declared native scale.", nil)
			continue
		}
		if paragraphContext && (sample.StartOffset != t.Start || sample.EndOffset != t.End || sample.Text != input.TargetText) {
			e.unknown(id, r, t, related, "measure.unmapped-sentence", "Provider did not return native attribution for the selected sentence.")
			continue
		}
		evidence := t
		if d.Granularity == "window" {
			if sample.StartOffset < t.Start || sample.EndOffset > t.End || sample.EndOffset <= sample.StartOffset || sample.Text == "" || string(e.document.Source[sample.StartOffset:sample.EndOffset]) != sample.Text {
				e.unknown(id, r, t, related, "measure.unmapped-window", "Provider window does not align with the original source.")
				continue
			}
			copyTarget := *t
			copyTarget.Start, copyTarget.End = sample.StartOffset, sample.EndOffset
			evidence = &copyTarget
		}
		value := sample.Value
		fail := false
		if min, ok := number(r.Options["min"]); ok && value < min {
			fail = true
		}
		if max, ok := number(r.Options["max"]); ok && value > max {
			fail = true
		}
		if fail {
			e.report.Summary.FailedTargets++
			before := len(e.report.Diagnostics)
			e.finding(id, r, evidence, "limit.out-of-range", fmt.Sprintf("%s is %g, outside the required bounds.", d.ID, value), related, map[string]any{"min": r.Options["min"], "max": r.Options["max"]}, value, []Measurement{{Name: d.ID, Value: value, Unit: d.Unit, Min: r.Options["min"], Max: r.Options["max"], Inputs: sample.Inputs, Sample: sample.Classification}})
			last := len(e.report.Diagnostics) - 1
			if last >= before {
				e.report.Diagnostics[last].Provenance["measureVersion"] = d.Version
				e.report.Diagnostics[last].Provenance["measure"] = d.ID
				e.report.Diagnostics[last].Provenance["provider"] = provider
			}
		}
	}
}
func (e *execution) unknown(id string, r Rule, t *Target, related []Related, code, message string) {
	e.report.Complete = false
	e.report.Summary.UnknownTargets++
	severity := r.Severity
	if r.Unknown == "report" {
		severity = "info"
	}
	before := len(e.report.Diagnostics)
	e.finding(id, Rule{Check: r.Check, Severity: severity}, t, code, message, related, nil, nil, nil)
	if len(e.report.Diagnostics) > before {
		e.report.Diagnostics[len(e.report.Diagnostics)-1].Result = "unknown"
	}
}
