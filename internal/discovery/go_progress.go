package discovery

import (
	"encoding/json"
	"go/token"
	"strings"
)

// GoJSONProgress assembles per-package output lines before reporting test names.
type GoJSONProgress struct {
	pending map[string]string
}

func (p *GoJSONProgress) Push(raw string) []string {
	var event struct {
		Action  string `json:"Action"`
		Package string `json:"Package"`
		Output  string `json:"Output"`
	}
	if json.Unmarshal([]byte(raw), &event) != nil || event.Action != "output" || event.Package == "" {
		return nil
	}
	if p.pending == nil {
		p.pending = make(map[string]string)
	}
	text := p.pending[event.Package] + event.Output
	var names []string
	for {
		line, rest, found := strings.Cut(text, "\n")
		if !found {
			break
		}
		name := strings.TrimSpace(line)
		if token.IsIdentifier(name) && (strings.HasPrefix(name, "Test") || strings.HasPrefix(name, "Example") || strings.HasPrefix(name, "Fuzz") || strings.HasPrefix(name, "Benchmark")) {
			names = append(names, name)
		}
		text = rest
	}
	p.pending[event.Package] = text
	return names
}
