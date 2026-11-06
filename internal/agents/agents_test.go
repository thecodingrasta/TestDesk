package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallProjectJSONTargets(t *testing.T) {
	project := t.TempDir()
	t.Setenv("HOME", project)
	t.Setenv("USERPROFILE", project)
	installer, err := NewInstaller(project, filepath.Join(project, "testdesk.exe"))
	if err != nil {
		t.Fatal(err)
	}

	for _, target := range []TargetID{TargetClaude, TargetOpenCode, TargetCursorGemini, TargetCline} {
		workspace, err := installer.Install(target)
		if err != nil {
			t.Fatalf("install %s: %v", target, err)
		}
		if !workspace.Installed {
			t.Fatalf("expected %s to report installed", target)
		}
	}
}

func TestGlobalMCPTargetsOmitProjectArg(t *testing.T) {
	installer, err := NewInstaller("", filepath.Join("C:", "TestDesk.exe"))
	if err != nil {
		t.Fatal(err)
	}
	manual := installer.Manual(TargetAntigravity)
	if !strings.Contains(manual, `"args": [`) || !strings.Contains(manual, `"mcp"`) {
		t.Fatalf("unexpected Antigravity config: %s", manual)
	}
	if strings.Contains(manual, "--project") {
		t.Fatalf("expected no --project arg in Antigravity manual config: %s", manual)
	}
	if strings.Contains(manual, `"cwd"`) || strings.Contains(manual, `"env"`) {
		t.Fatalf("expected empty cwd and env to be omitted from Antigravity manual config: %s", manual)
	}

	codex := installer.Manual(TargetCodex)
	if !strings.Contains(codex, `args = ["mcp"]`) {
		t.Fatalf("unexpected Codex config: %s", codex)
	}
	if strings.Contains(codex, "--project") || strings.Contains(codex, "cwd =") {
		t.Fatalf("expected no --project or cwd in Codex manual config: %s", codex)
	}
}

func TestRemoveTomlTable(t *testing.T) {
	input := `model = "gpt-5"

[mcp_servers.testdesk]
command = "old"
args = []

[mcp_servers.other]
command = "keep"
`
	output := removeTomlTable(input, "mcp_servers.testdesk")
	if strings.Contains(output, "command = \"old\"") {
		t.Fatal("old TestDesk table was not removed")
	}
	if !strings.Contains(output, "[mcp_servers.other]") {
		t.Fatal("other MCP server table was removed")
	}
}

func TestInstallBacksUpExistingConfig(t *testing.T) {
	project := t.TempDir()
	path := filepath.Join(project, ".mcp.json")
	if err := os.WriteFile(path, []byte(`{"mcpServers":{"other":{"command":"keep"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	installer, err := NewInstaller(project, filepath.Join(project, "testdesk.exe"))
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := installer.Install(TargetClaudeCode)
	if err != nil {
		t.Fatal(err)
	}
	if workspace.BackupPath == "" {
		t.Fatal("installer did not report the backup path")
	}

	backups, err := filepath.Glob(path + ".testdesk-backup-*")
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 {
		t.Fatalf("expected one backup, got %v", backups)
	}
	data, err := os.ReadFile(backups[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"other"`) {
		t.Fatalf("backup did not preserve prior configuration: %s", data)
	}
}

func TestInstallReportsInstructionFailureAfterMCPConfig(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, ".agents"), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	installer, err := NewInstaller(project, filepath.Join(project, "testdesk.exe"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = installer.Install(TargetClaudeCode)
	if err == nil || !strings.Contains(err.Error(), "MCP configuration was installed") {
		t.Fatalf("expected explicit partial-install error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, ".mcp.json")); err != nil {
		t.Fatalf("MCP configuration should have been installed: %v", err)
	}
}
