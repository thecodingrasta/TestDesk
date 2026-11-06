// Command sync-agent-assets keeps published harness instructions in sync with
// the templates shipped by the application. It never installs MCP credentials.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/thecodingrasta/testdesk/internal/agents"
)

func main() {
	check := flag.Bool("check", false, "check without writing files")
	flag.Parse()
	if _, err := os.Stat("go.mod"); err != nil {
		fmt.Fprintln(os.Stderr, "run from the repository root")
		os.Exit(1)
	}
	files := map[string]string{
		"CLAUDE.md":                        agents.AgentsInstructionBlock,
		"GEMINI.md":                        agents.AgentsInstructionBlock,
		".github/copilot-instructions.md":  agents.AgentsInstructionBlock,
		".cursor/rules/testdesk.mdc":       agents.CursorRuleContent,
		".gemini/skills/testdesk/SKILL.md": agents.SkillGuideContent,
	}
	skills := map[string]string{
		"testdesk-guide":     agents.SkillGuideContent,
		"testdesk-running":   agents.SkillRunningContent,
		"testdesk-debugging": agents.SkillDebuggingContent,
		"testdesk-projects":  agents.SkillProjectsContent,
	}
	for name, content := range skills {
		for _, base := range []string{".agents/skills", ".claude/skills"} {
			files[filepath.Join(base, name, "SKILL.md")] = content
		}
	}
	failed := false
	for name, content := range files {
		expected := []byte(strings.TrimSpace(content) + "\n")
		if *check {
			actual, err := os.ReadFile(name)
			if err != nil || !bytes.Equal(actual, expected) {
				fmt.Fprintln(os.Stderr, "outdated agent asset:", name)
				failed = true
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := os.WriteFile(name, expected, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if failed {
		os.Exit(1)
	}
}
