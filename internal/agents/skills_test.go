package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallSkills(t *testing.T) {
	project := t.TempDir()

	// Initial install
	result, err := InstallSkills(project)
	if err != nil {
		t.Fatalf("InstallSkills error: %v", err)
	}
	if len(result.InstalledFiles) == 0 {
		t.Fatal("expected files to be installed")
	}

	// Verify AGENTS.md exists and contains the TestDesk block
	agentsPath := filepath.Join(project, "AGENTS.md")
	data, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if !strings.Contains(string(data), BlockStartTag) || !strings.Contains(string(data), BlockEndTag) {
		t.Fatalf("AGENTS.md missing block tags: %s", data)
	}
	if !strings.Contains(string(data), "testdesk_run") || !strings.Contains(string(data), "testdesk_rerun_failed") {
		t.Fatalf("AGENTS.md missing key tools: %s", data)
	}

	// Verify CLAUDE.md exists
	claudePath := filepath.Join(project, "CLAUDE.md")
	claudeData, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	if !strings.Contains(string(claudeData), "TestDesk") {
		t.Fatalf("CLAUDE.md missing TestDesk content: %s", claudeData)
	}

	// Verify skill files exist
	guidePath := filepath.Join(project, ".claude", "skills", "testdesk-guide", "SKILL.md")
	if _, err := os.Stat(guidePath); err != nil {
		t.Fatalf("expected guide skill at %s: %v", guidePath, err)
	}

	cursorPath := filepath.Join(project, ".cursor", "rules", "testdesk.mdc")
	if _, err := os.Stat(cursorPath); err != nil {
		t.Fatalf("expected cursor rules at %s: %v", cursorPath, err)
	}

	geminiPath := filepath.Join(project, ".gemini", "skills", "testdesk", "SKILL.md")
	if _, err := os.Stat(geminiPath); err != nil {
		t.Fatalf("expected gemini skill at %s: %v", geminiPath, err)
	}
	copilotPath := filepath.Join(project, ".github", "copilot-instructions.md")
	if _, err := os.Stat(copilotPath); err != nil {
		t.Fatalf("expected Copilot instructions: %v", err)
	}
	for _, name := range []string{"testdesk-guide", "testdesk-running", "testdesk-debugging", "testdesk-projects"} {
		for _, base := range []string{".agents/skills", ".claude/skills"} {
			path := filepath.Join(project, filepath.FromSlash(base), name, "SKILL.md")
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("expected skill at %s: %v", path, err)
			}
		}
	}

	// Re-install should update cleanly without duplicating content
	secondResult, err := InstallSkills(project)
	if err != nil {
		t.Fatalf("second InstallSkills error: %v", err)
	}
	if len(secondResult.UpdatedFiles) == 0 {
		t.Fatal("expected files to be reported as updated on re-run")
	}

	dataAfter, _ := os.ReadFile(agentsPath)
	if strings.Count(string(dataAfter), BlockStartTag) != 1 {
		t.Fatalf("expected exactly 1 start tag in AGENTS.md, got %d", strings.Count(string(dataAfter), BlockStartTag))
	}
}

func TestUpdateDelimitedBlockPreservesSurroundingContent(t *testing.T) {
	initial := `# Custom Project Rules
Do not delete database in tests.

<!-- testdesk:start -->
Old testdesk instructions
<!-- testdesk:end -->

<!-- gitnexus:start -->
GitNexus instructions
<!-- gitnexus:end -->
`
	newBlock := `<!-- testdesk:start -->
New improved instructions
<!-- testdesk:end -->`

	updated := updateDelimitedBlock(initial, BlockStartTag, BlockEndTag, newBlock)
	if !strings.Contains(updated, "Custom Project Rules") {
		t.Fatal("custom project rules were removed")
	}
	if !strings.Contains(updated, "GitNexus instructions") {
		t.Fatal("gitnexus instructions were removed")
	}
	if !strings.Contains(updated, "New improved instructions") {
		t.Fatal("new instructions not found")
	}
	if strings.Contains(updated, "Old testdesk instructions") {
		t.Fatal("old instructions were not replaced")
	}
	if strings.Count(updated, BlockStartTag) != 1 {
		t.Fatalf("expected 1 testdesk start tag, found %d", strings.Count(updated, BlockStartTag))
	}
}

func TestInstallSkillsRejectsEmptyRoot(t *testing.T) {
	if _, err := InstallSkills(" "); err == nil {
		t.Fatal("expected empty project root to fail")
	}
}

func TestInstallSkillsBacksUpExistingSkill(t *testing.T) {
	project := t.TempDir()
	path := filepath.Join(project, ".agents", "skills", "testdesk-guide", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	original := []byte("user authored skill\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallSkills(project); err != nil {
		t.Fatal(err)
	}
	backups, err := filepath.Glob(path + ".testdesk-backup-*")
	if err != nil || len(backups) != 1 {
		t.Fatalf("expected one backup, got %v (err %v)", backups, err)
	}
	data, err := os.ReadFile(backups[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(original) {
		t.Fatalf("backup mismatch: %q", data)
	}
}
