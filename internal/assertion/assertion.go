package assertion

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

type Type string

const (
	SerialContains    Type = "serial_contains"
	SerialNotContains Type = "serial_not_contains"
)

type Spec struct {
	Index         int
	Type          Type
	Value         string
	Within        time.Duration
	CaseSensitive bool
}

type Result struct {
	Index    int
	Type     Type
	Value    string
	Passed   bool
	Duration time.Duration
	Message  string
	Evidence string
}

type Evaluator struct {
	specs     []Spec
	results   []Result
	done      []bool
	startedAt time.Time
	buffer    string
	maxBytes  int
}

func NewEvaluator(specs []Spec, startedAt time.Time, maxBytes int) *Evaluator {
	if maxBytes <= 0 {
		maxBytes = 1024 * 1024
	}
	maxNeedle := 0
	for _, spec := range specs {
		if len(spec.Value) > maxNeedle {
			maxNeedle = len(spec.Value)
		}
	}
	if maxBytes < maxNeedle*2 {
		maxBytes = maxNeedle * 2
	}
	return &Evaluator{
		specs:     append([]Spec(nil), specs...),
		done:      make([]bool, len(specs)),
		startedAt: startedAt,
		maxBytes:  maxBytes,
	}
}

func (e *Evaluator) Observe(chunk string, at time.Time) []Result {
	if chunk == "" {
		return nil
	}
	e.buffer += chunk
	if len(e.buffer) > e.maxBytes {
		e.buffer = e.buffer[len(e.buffer)-e.maxBytes:]
		for !utf8.ValidString(e.buffer) && len(e.buffer) > 0 {
			e.buffer = e.buffer[1:]
		}
	}

	elapsed := at.Sub(e.startedAt)
	var completed []Result
	for i, spec := range e.specs {
		if e.done[i] {
			continue
		}
		switch spec.Type {
		case SerialContains:
			if elapsed <= spec.Within && contains(e.buffer, spec.Value, spec.CaseSensitive) {
				result := Result{
					Index:    spec.Index,
					Type:     spec.Type,
					Value:    spec.Value,
					Passed:   true,
					Duration: elapsed,
					Message:  fmt.Sprintf("Found %q", spec.Value),
					Evidence: snippetAround(e.buffer, spec.Value, spec.CaseSensitive),
				}
				e.complete(i, result)
				completed = append(completed, result)
			}
		case SerialNotContains:
			if contains(e.buffer, spec.Value, spec.CaseSensitive) {
				result := Result{
					Index:    spec.Index,
					Type:     spec.Type,
					Value:    spec.Value,
					Passed:   false,
					Duration: elapsed,
					Message:  fmt.Sprintf("Observed forbidden %q", spec.Value),
					Evidence: snippetAround(e.buffer, spec.Value, spec.CaseSensitive),
				}
				e.complete(i, result)
				completed = append(completed, result)
			}
		}
	}
	return completed
}

func (e *Evaluator) Finish(at time.Time) []Result {
	elapsed := at.Sub(e.startedAt)
	var completed []Result
	for i, spec := range e.specs {
		if e.done[i] {
			continue
		}
		switch spec.Type {
		case SerialContains:
			deadline := spec.Within
			result := Result{
				Index:    spec.Index,
				Type:     spec.Type,
				Value:    spec.Value,
				Passed:   false,
				Duration: deadline,
				Message:  fmt.Sprintf("Expected %q not observed within %s", spec.Value, deadline),
			}
			e.complete(i, result)
			completed = append(completed, result)
		case SerialNotContains:
			result := Result{
				Index:    spec.Index,
				Type:     spec.Type,
				Value:    spec.Value,
				Passed:   true,
				Duration: elapsed,
				Message:  fmt.Sprintf("Did not observe %q", spec.Value),
			}
			e.complete(i, result)
			completed = append(completed, result)
		}
	}
	return completed
}

func (e *Evaluator) Results() []Result {
	return append([]Result(nil), e.results...)
}

func (e *Evaluator) complete(i int, result Result) {
	e.done[i] = true
	e.results = append(e.results, result)
}

func contains(haystack, needle string, caseSensitive bool) bool {
	if caseSensitive {
		return strings.Contains(haystack, needle)
	}
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}

func snippetAround(haystack, needle string, caseSensitive bool) string {
	searchHaystack := haystack
	searchNeedle := needle
	if !caseSensitive {
		searchHaystack = strings.ToLower(haystack)
		searchNeedle = strings.ToLower(needle)
	}
	idx := strings.Index(searchHaystack, searchNeedle)
	if idx < 0 {
		return ""
	}
	start := idx - 80
	if start < 0 {
		start = 0
	}
	end := idx + len(needle) + 80
	if end > len(haystack) {
		end = len(haystack)
	}
	return strings.TrimSpace(haystack[start:end])
}
