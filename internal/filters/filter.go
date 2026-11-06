package filters

import (
	"strings"

	"github.com/thecodingrasta/testdesk/internal/domain"
)

// Apply filters tests and returns total before pagination plus selected page.
func Apply(tests []domain.TestCase, f domain.Filter) (int, []domain.TestCase) {
	matched := make([]domain.TestCase, 0, len(tests))
	for _, test := range tests {
		if !matchAny(test.ProviderID, f.ProviderIDs) {
			continue
		}
		if !matchAny(string(test.ProviderKind), f.ProviderKinds) {
			continue
		}
		if !matchAny(test.Layer, f.Layers) {
			continue
		}
		if !matchAny(test.Module, f.Modules) {
			continue
		}
		if len(f.Statuses) > 0 && !statusIn(test.LastStatus, f.Statuses) {
			continue
		}
		if len(f.Tags) > 0 && !hasAllTags(test.Tags, f.Tags) {
			continue
		}
		if strings.TrimSpace(f.Query) != "" && !containsText(test, f.Query) {
			continue
		}
		matched = append(matched, test)
	}
	total := len(matched)
	if f.Offset < 0 {
		f.Offset = 0
	}
	if f.Offset >= len(matched) {
		return total, []domain.TestCase{}
	}
	end := len(matched)
	if f.Limit > 0 && f.Offset+f.Limit < end {
		end = f.Offset + f.Limit
	}
	return total, matched[f.Offset:end]
}

func matchAny(value string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, item := range allowed {
		if strings.EqualFold(value, item) {
			return true
		}
	}
	return false
}

func statusIn(value domain.Status, allowed []domain.Status) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}

func hasAllTags(existing []string, required []string) bool {
	set := make(map[string]struct{}, len(existing))
	for _, tag := range existing {
		set[strings.ToLower(tag)] = struct{}{}
	}
	for _, tag := range required {
		if _, ok := set[strings.ToLower(tag)]; !ok {
			return false
		}
	}
	return true
}

func containsText(test domain.TestCase, query string) bool {
	needle := strings.ToLower(strings.TrimSpace(query))
	blob := strings.ToLower(test.Name + " " + test.FullName + " " + test.Selector + " " + test.File + " " + test.Module + " " + strings.Join(test.Tags, " "))
	return strings.Contains(blob, needle)
}
