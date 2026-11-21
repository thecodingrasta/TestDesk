// Command sync-agent-assets synchronizes canonical Agent Skills, progressive
// references, embedded binary templates, published marketplace copies, and root
// licence notices from agent-distribution/testdesk/plugin/skills.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/thecodingrasta/testdesk/internal/agents"
)

const canonicalSkillsDir = "agent-distribution/testdesk/plugin/skills"

func main() {
	check := flag.Bool("check", false, "check without writing files")
	flag.Parse()
	if _, err := os.Stat("go.mod"); err != nil {
		fmt.Fprintln(os.Stderr, "run from the repository root")
		os.Exit(1)
	}
	expected := map[string][]byte{
		"AGENTS.md":                       norm(agents.AgentsInstructionBlock),
		"CLAUDE.md":                       norm(agents.AgentsInstructionBlock),
		"GEMINI.md":                       norm(agents.AgentsInstructionBlock),
		".github/copilot-instructions.md": norm(agents.AgentsInstructionBlock),
		".cursor/rules/testdesk.mdc":      norm(agents.CursorRuleContent),
	}

	canonical := map[string][]byte{}
	err := filepath.WalkDir(canonicalSkillsDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		rel, relErr := filepath.Rel(canonicalSkillsDir, path)
		if relErr != nil {
			return relErr
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		canonical[filepath.ToSlash(rel)] = normalizeBytes(body)
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "read canonical skills:", err)
		os.Exit(1)
	}
	if len(canonical) == 0 {
		fmt.Fprintln(os.Stderr, "canonical skills tree is empty:", canonicalSkillsDir)
		os.Exit(1)
	}

	skillRoots := []string{
		"internal/agents/embedded/skills",
		".agents/skills",
		".claude/skills",
		"agent-distribution/testdesk/adapters/claude-code/skills",
		"agent-distribution/testdesk/adapters/cursor/skills",
	}
	for rel, body := range canonical {
		for _, base := range skillRoots {
			expected[filepath.ToSlash(filepath.Join(base, filepath.FromSlash(rel)))] = body
		}
		if strings.HasPrefix(rel, "testdesk-guide/") {
			geminiRel := strings.TrimPrefix(rel, "testdesk-guide/")
			expected[filepath.ToSlash(filepath.Join(".gemini/skills/testdesk", filepath.FromSlash(geminiRel)))] = body
		}
	}

	for _, notice := range []string{"LICENSE"} {
		body, readErr := os.ReadFile(notice)
		if readErr != nil {
			fmt.Fprintln(os.Stderr, "read notice", notice+":", readErr)
			os.Exit(1)
		}
		for _, source := range []string{
			"agent-distribution/testdesk/plugin",
			"agent-distribution/testdesk/adapters/claude-code",
			"agent-distribution/testdesk/adapters/cursor",
		} {
			expected[filepath.ToSlash(filepath.Join(source, notice))] = body
		}
	}

	// Ensure no stale extra files linger inside synchronized skill trees.
	for _, tree := range append(append([]string{}, skillRoots...), ".gemini/skills/testdesk") {
		if _, statErr := os.Stat(tree); statErr != nil {
			continue
		}
		_ = filepath.WalkDir(tree, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil || entry.IsDir() {
				return walkErr
			}
			posix := filepath.ToSlash(path)
			if _, ok := expected[posix]; !ok {
				if *check {
					fmt.Fprintln(os.Stderr, "unexpected stale agent asset:", posix)
					os.Exit(1)
				}
				_ = os.Remove(path)
			}
			return nil
		})
	}

	paths := make([]string, 0, len(expected))
	for p := range expected {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	failed := false
	for _, name := range paths {
		want := expected[name]
		if *check {
			actual, readErr := os.ReadFile(name)
			if readErr != nil || !bytes.Equal(actual, want) {
				fmt.Fprintln(os.Stderr, "outdated agent asset:", name)
				failed = true
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := os.WriteFile(name, want, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if failed {
		os.Exit(1)
	}
}

func norm(s string) []byte {
	return normalizeBytes([]byte(s))
}

func normalizeBytes(b []byte) []byte {
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	return []byte(strings.TrimSpace(s) + "\n")
}
