package agents

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed embedded/skills
var embeddedSkills embed.FS

const (
	BlockStartTag = "<!-- testdesk:start -->"
	BlockEndTag   = "<!-- testdesk:end -->"
)

type SkillInstallResult struct {
	ProjectRoot    string   `json:"projectRoot"`
	InstalledFiles []string `json:"installedFiles"`
	UpdatedFiles   []string `json:"updatedFiles"`
	Message        string   `json:"message"`
}

const AgentsInstructionBlock = `<!-- testdesk:start -->
# TestDesk

Use TestDesk MCP for configured project tests. Inspect ` + "`" + `testdesk_capabilities` + "`" + ` or ` + "`" + `testdesk_snapshot` + "`" + ` before queueing unfamiliar work. A known native test selector can run directly when orchestration adds no value.

- Find test IDs with ` + "`" + `testdesk_tests` + "`" + `; narrow by project, provider, layer, module or query.
- Pass an explicit ` + "`" + `project` + "`" + ` name or absolute root when working across repositories.
- Preview broad suites with ` + "`" + `testdesk_plan` + "`" + `; prefer explicit IDs for focused checks.
- Queue with ` + "`" + `testdesk_run` + "`" + `, wait with ` + "`" + `testdesk_wait` + "`" + `, then inspect status.
- On failure, request ` + "`" + `testdesk_diagnostics` + "`" + ` with ` + "`" + `maxBytes: 4096` + "`" + ` and use ` + "`" + `testdesk_test_details` + "`" + ` for identity and failure evidence. Redact sensitive output before sharing.
- After a fix, use ` + "`" + `testdesk_rerun_failed` + "`" + ` for that project. Do not stop another client's work without authorization.
- Provider configuration runs local commands with user permissions. Review untrusted configurations before running or installing instructions.

Read only the skill needed:

| Task | Skill |
|---|---|
| Setup and workflow | ` + "`" + `.agents/skills/testdesk-guide/SKILL.md` + "`" + ` |
| Plan, run and control | ` + "`" + `.agents/skills/testdesk-running/SKILL.md` + "`" + ` |
| Diagnose failures | ` + "`" + `.agents/skills/testdesk-debugging/SKILL.md` + "`" + ` |
| Configure multiple projects | ` + "`" + `.agents/skills/testdesk-projects/SKILL.md` + "`" + ` |

<!-- testdesk:end -->`

var (
	SkillGuideContent     = mustReadEmbeddedSkill("embedded/skills/testdesk-guide/SKILL.md")
	SkillRunningContent   = mustReadEmbeddedSkill("embedded/skills/testdesk-running/SKILL.md")
	SkillDebuggingContent = mustReadEmbeddedSkill("embedded/skills/testdesk-debugging/SKILL.md")
	SkillProjectsContent  = mustReadEmbeddedSkill("embedded/skills/testdesk-projects/SKILL.md")
)

func mustReadEmbeddedSkill(name string) string {
	data, err := embeddedSkills.ReadFile(name)
	if err != nil {
		panic(fmt.Sprintf("read embedded skill %s: %v", name, err))
	}
	return strings.ReplaceAll(string(data), "\r\n", "\n")
}

// EmbeddedSkillFiles returns every canonical skill and reference file keyed by path relative to the skills root.
func EmbeddedSkillFiles() (map[string]string, error) {
	files := map[string]string{}
	err := fs.WalkDir(embeddedSkills, "embedded/skills", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}
		data, err := embeddedSkills.ReadFile(path)
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(path, "embedded/skills/")
		files[rel] = strings.ReplaceAll(string(data), "\r\n", "\n")
		return nil
	})
	return files, err
}

const CursorRuleContent = `---
description: TestDesk test runner and orchestration guidelines
globs: *
alwaysApply: true
---

# TestDesk Test Orchestrator Rules

When running, planning, or debugging tests in this workspace:
- Use TestDesk MCP tools (` + "`testdesk_snapshot`" + `, ` + "`testdesk_tests`" + `, ` + "`testdesk_run`" + `, ` + "`testdesk_rerun_failed`" + `, ` + "`testdesk_diagnostics`" + `).
- Use ` + "`testdesk_plan`" + ` before broad test executions.
- Use ` + "`testdesk_rerun_failed`" + ` after fixing broken tests.
- Bound diagnostics requests (` + "`maxBytes: 4096`" + `) rather than reading full log streams.
`

func InstallSkills(projectRoot string) (SkillInstallResult, error) {
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot == "" {
		return SkillInstallResult{}, errors.New("project root is required to install skills")
	}
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return SkillInstallResult{}, err
	}

	result := SkillInstallResult{
		ProjectRoot:    root,
		InstalledFiles: []string{},
		UpdatedFiles:   []string{},
	}

	agentsPath := filepath.Join(root, "AGENTS.md")
	if updated, err := updateOrAppendDelimitedFile(agentsPath, AgentsInstructionBlock); err != nil {
		return result, fmt.Errorf("update AGENTS.md: %w", err)
	} else if updated {
		result.UpdatedFiles = append(result.UpdatedFiles, "AGENTS.md")
	} else {
		result.InstalledFiles = append(result.InstalledFiles, "AGENTS.md")
	}

	claudePath := filepath.Join(root, "CLAUDE.md")
	if updated, err := updateOrAppendDelimitedFile(claudePath, AgentsInstructionBlock); err != nil {
		return result, fmt.Errorf("update CLAUDE.md: %w", err)
	} else if updated {
		result.UpdatedFiles = append(result.UpdatedFiles, "CLAUDE.md")
	} else {
		result.InstalledFiles = append(result.InstalledFiles, "CLAUDE.md")
	}

	// Install skills and progressive references in each harness's discoverable location.
	embeddedFiles, err := EmbeddedSkillFiles()
	if err != nil {
		return result, fmt.Errorf("load embedded skills: %w", err)
	}
	relPaths := make([]string, 0, len(embeddedFiles))
	for rel := range embeddedFiles {
		relPaths = append(relPaths, rel)
	}
	sort.Strings(relPaths)
	for _, rel := range relPaths {
		content := embeddedFiles[rel]
		for _, base := range []string{filepath.Join(".agents", "skills"), filepath.Join(".claude", "skills")} {
			if err := installSkillFile(root, filepath.Join(base, filepath.FromSlash(rel)), content, &result); err != nil {
				return result, err
			}
		}
		if strings.HasPrefix(rel, "testdesk-guide/") {
			geminiRel := strings.TrimPrefix(rel, "testdesk-guide/")
			if err := installSkillFile(root, filepath.Join(".gemini", "skills", "testdesk", filepath.FromSlash(geminiRel)), content, &result); err != nil {
				return result, err
			}
		}
	}

	if err := installSkillFile(root, filepath.Join(".cursor", "rules", "testdesk.mdc"), CursorRuleContent, &result); err != nil {
		return result, err
	}
	if err := updateInstructionFile(filepath.Join(root, "GEMINI.md"), &result); err != nil {
		return result, fmt.Errorf("update GEMINI.md: %w", err)
	}
	if err := updateInstructionFile(filepath.Join(root, ".github", "copilot-instructions.md"), &result); err != nil {
		return result, fmt.Errorf("update Copilot instructions: %w", err)
	}

	total := len(result.InstalledFiles) + len(result.UpdatedFiles)
	result.Message = fmt.Sprintf("Successfully installed/updated TestDesk skills across %d files in %s.", total, filepath.Base(root))
	return result, nil
}

func updateInstructionFile(path string, result *SkillInstallResult) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	updated, err := updateOrAppendDelimitedFile(path, AgentsInstructionBlock)
	if err != nil {
		return err
	}
	name, _ := filepath.Rel(result.ProjectRoot, path)
	if updated {
		result.UpdatedFiles = append(result.UpdatedFiles, name)
	} else {
		result.InstalledFiles = append(result.InstalledFiles, name)
	}
	return nil
}

func installSkillFile(root, relPath, content string, result *SkillInstallResult) error {
	path := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	isNew := errors.Is(err, os.ErrNotExist)
	if err != nil && !isNew {
		return fmt.Errorf("read %s: %w", relPath, err)
	}
	newData := []byte(strings.TrimSpace(content) + "\n")
	if !isNew && string(data) == string(newData) {
		result.UpdatedFiles = append(result.UpdatedFiles, relPath)
		return nil
	}
	if !isNew {
		var random [8]byte
		if _, err := rand.Read(random[:]); err != nil {
			return err
		}
		backup := path + ".testdesk-backup-" + hex.EncodeToString(random[:])
		if err := os.WriteFile(backup, data, 0o600); err != nil {
			return fmt.Errorf("back up %s: %w", relPath, err)
		}
	}
	if err := os.WriteFile(path, newData, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", relPath, err)
	}
	if isNew {
		result.InstalledFiles = append(result.InstalledFiles, relPath)
	} else {
		result.UpdatedFiles = append(result.UpdatedFiles, relPath)
	}
	return nil
}

func updateOrAppendDelimitedFile(path string, newBlock string) (isUpdated bool, err error) {
	data, readErr := os.ReadFile(path)
	if errors.Is(readErr, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return false, err
		}
		return false, os.WriteFile(path, []byte(strings.TrimSpace(newBlock)+"\n"), 0o644)
	}
	if readErr != nil {
		return false, readErr
	}

	content := string(data)
	updatedContent := updateDelimitedBlock(content, BlockStartTag, BlockEndTag, newBlock)
	if updatedContent == content {
		return true, nil
	}
	return true, os.WriteFile(path, []byte(updatedContent), 0o644)
}

func updateDelimitedBlock(content string, startTag string, endTag string, newBlock string) string {
	startIdx := strings.Index(content, startTag)
	endIdx := strings.Index(content, endTag)

	if startIdx != -1 && endIdx != -1 && endIdx > startIdx {
		before := strings.TrimRight(content[:startIdx], "\r\n ")
		after := strings.TrimLeft(content[endIdx+len(endTag):], "\r\n ")
		var out strings.Builder
		if before != "" {
			out.WriteString(before)
			out.WriteString("\n\n")
		}
		out.WriteString(strings.TrimSpace(newBlock))
		if after != "" {
			out.WriteString("\n\n")
			out.WriteString(after)
		}
		out.WriteString("\n")
		return out.String()
	}

	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return strings.TrimSpace(newBlock) + "\n"
	}
	return trimmed + "\n\n" + strings.TrimSpace(newBlock) + "\n"
}
