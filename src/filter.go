package tape

import (
	"fmt"
	"strings"
	"time"
)

type Filter struct {
	Symbols    []string
	EventTypes []string
	StartTime  time.Time
	EndTime    time.Time
}

func (f Filter) normalized() Filter {
	f.Symbols = normalizeFilterValues(f.Symbols, normalizeSymbol)
	f.EventTypes = normalizeFilterValues(f.EventTypes, normalizeEventType)
	return f
}

func (f Filter) validate() error {
	if !f.StartTime.IsZero() && !f.EndTime.IsZero() && f.StartTime.After(f.EndTime) {
		return fmt.Errorf(
			"config error: filter start time %s is after end time %s",
			f.StartTime.Format(time.RFC3339Nano),
			f.EndTime.Format(time.RFC3339Nano),
		)
	}
	return nil
}

func (f Filter) Matches(event Event) bool {
	if len(f.Symbols) > 0 && !containsNormalizedValue(f.Symbols, event.Symbol(), normalizeSymbol) {
		return false
	}
	if len(f.EventTypes) > 0 && !containsNormalizedValue(f.EventTypes, event.Type(), normalizeEventType) {
		return false
	}

	timestamp := event.Timestamp()
	if !f.StartTime.IsZero() && timestamp.Before(f.StartTime) {
		return false
	}
	if !f.EndTime.IsZero() && timestamp.After(f.EndTime) {
		return false
	}

	return true
}

func normalizeFilterValues(values []string, normalize func(string) string) []string {
	if len(values) == 0 {
		return nil
	}

	seen := map[string]struct{}{}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		candidate := normalize(value)
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		normalized = append(normalized, candidate)
	}

	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func containsNormalizedValue(values []string, candidate string, normalize func(string) string) bool {
	normalizedCandidate := normalize(candidate)
	for _, value := range values {
		if normalize(value) == normalizedCandidate {
			return true
		}
	}
	return false
}

func normalizeSymbol(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func normalizeEventType(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
