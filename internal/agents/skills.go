package agents

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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

const SkillGuideContent = `---
name: testdesk-guide
description: Use TestDesk MCP to inspect capabilities, discover test IDs and choose a focused test workflow for a configured local project.
---

# TestDesk workflow

Check ` + "`" + `testdesk_capabilities` + "`" + ` and a compact ` + "`" + `testdesk_snapshot` + "`" + ` before unfamiliar runs. Search ` + "`" + `testdesk_tests` + "`" + ` with a narrow query; retain returned IDs rather than inventing selectors. Include ` + "`" + `project` + "`" + ` when targeting a particular repository.

Plan broad runs before execution. Queue a focused selection, wait for completion and inspect bounded diagnostics on failure. A plan's item count reflects execution units, which may be individual tests, files or a whole provider. It is not always a test count.

TestDesk uses installed test runners and trusted local configuration. A provider template is not a guarantee of compatibility with every version or platform. Use ` + "`" + `testdesk_settings` + "`" + ` to inspect the actual commands. Do not install frameworks or edit project configuration merely to make a test green.`

const SkillRunningContent = `---
name: testdesk-running
description: Plan, queue, wait for and control TestDesk test runs using explicit test IDs or narrow filters.
---

# Run tests

1. Inspect the selected project's state with ` + "`" + `testdesk_snapshot` + "`" + `.
2. Find IDs with ` + "`" + `testdesk_tests` + "`" + `, scoped by ` + "`" + `project` + "`" + `, ` + "`" + `providerIds` + "`" + `, ` + "`" + `layers` + "`" + `, ` + "`" + `modules` + "`" + ` or ` + "`" + `query` + "`" + ` as needed.
3. Use ` + "`" + `testdesk_plan` + "`" + ` before broad scopes. Inspect provider-wide items and overlap warnings.
4. Call ` + "`" + `testdesk_run` + "`" + ` with IDs returned by discovery. Combined filters constrain every requested ID; contradictory selections are rejected.
5. Call ` + "`" + `testdesk_wait` + "`" + ` with ` + "`" + `timeoutMs: 30000` + "`" + `, then check ` + "`" + `testdesk_run_status` + "`" + `. A wait timeout does not cancel execution.
6. Request bounded diagnostics for a failing run and rerun the failed subset after fixing it.

` + "`" + `testdesk_control` + "`" + ` accepts ` + "`" + `pause` + "`" + `, ` + "`" + `resume` + "`" + `, ` + "`" + `stop` + "`" + `, ` + "`" + `kill` + "`" + ` and ` + "`" + `skip` + "`" + `. Pause holds the queue; stop cancels current work and queued runs. Stop and kill currently share the manager cancellation path, so kill is not an independent escalation mechanism. Scope controls to the intended project and do not interrupt unrelated work.`

const SkillDebuggingContent = `---
name: testdesk-debugging
description: Diagnose a failed TestDesk run using bounded logs, test identity, result evidence and a focused rerun after a fix.
---

# Diagnose failures

Find the failing run with ` + "`" + `testdesk_history` + "`" + ` using ` + "`" + `status: "failed"` + "`" + ` and a small ` + "`" + `limit` + "`" + `. Request ` + "`" + `testdesk_diagnostics` + "`" + ` with its ` + "`" + `runId` + "`" + `, the intended ` + "`" + `project` + "`" + ` and ` + "`" + `maxBytes: 4096` + "`" + `.

Use ` + "`" + `testdesk_test_details` + "`" + ` with the catalog's ` + "`" + `testId` + "`" + ` when names are ambiguous. Use ` + "`" + `testdesk_artifacts` + "`" + ` for report references. Missing or malformed result evidence can make a run fail while the selected case remains unknown; do not turn unknown into a passing result.

Identify the cause before editing. After a fix, call ` + "`" + `testdesk_rerun_failed` + "`" + ` with the same project and run ID. Wait for its result. Keep the checks focused, and redact logs and environment values before sharing them.`

const SkillProjectsContent = `---
name: testdesk-projects
description: Register, select and configure TestDesk projects without changing another agent session's default or test work.
---

# Multiple projects

List projects with ` + "`" + `testdesk_projects` + "`" + `; use ` + "`" + `limit` + "`" + ` and ` + "`" + `offset` + "`" + ` for additional pages. Target a unique name or absolute root with the ` + "`" + `project` + "`" + ` argument on project-scoped tools. ` + "`" + `testdesk_project_activate` + "`" + ` changes this MCP session's default only.

Use ` + "`" + `testdesk_project_add` + "`" + ` with an absolute ` + "`" + `projectRoot` + "`" + ` to register a trusted repository. It creates ` + "`" + `testdesk.json` + "`" + ` only if absent. Review detected commands before running them.

For an idle project, ` + "`" + `testdesk_project_configure` + "`" + ` merges detected providers while preserving custom providers. Use ` + "`" + `replaceProviders` + "`" + ` only for an authorized full reset. Configuration is rejected while work is queued or running.

Queues and history are separate for each project. Avoid duplicate providers for the same test scope; inspect overlap warnings in execution plans. ` + "`" + `testdesk_project_forget_missing` + "`" + ` removes one explicitly selected missing, unloaded root from the registry without deleting files.`

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

	// Install skills in each harness's discoverable location.
	skills := []struct{ name, content string }{
		{"testdesk-guide", SkillGuideContent}, {"testdesk-running", SkillRunningContent},
		{"testdesk-debugging", SkillDebuggingContent}, {"testdesk-projects", SkillProjectsContent},
	}
	for _, sk := range skills {
		for _, rel := range []string{filepath.Join(".agents", "skills", sk.name, "SKILL.md"), filepath.Join(".claude", "skills", sk.name, "SKILL.md")} {
			if err := installSkillFile(root, rel, sk.content, &result); err != nil {
				return result, err
			}
		}
	}

	if err := installSkillFile(root, filepath.Join(".cursor", "rules", "testdesk.mdc"), CursorRuleContent, &result); err != nil {
		return result, err
	}

	if err := installSkillFile(root, filepath.Join(".gemini", "skills", "testdesk", "SKILL.md"), SkillGuideContent, &result); err != nil {
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
