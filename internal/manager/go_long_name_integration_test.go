package manager_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
	"github.com/thecodingrasta/testdesk/internal/runner"
)

func TestRealGoLongNameDiscoveryAndSelection(t *testing.T) {
	root := t.TempDir()
	name := "Test" + strings.Repeat("Long", 500)
	source := "package fixture\nimport (\"os\"; \"testing\")\n" +
		"func " + name + "(t *testing.T) { if err := os.WriteFile(\"selected\", []byte(\"yes\"), 0600); err != nil { t.Fatal(err) } }\n" +
		"func TestOther(t *testing.T) { if err := os.WriteFile(\"other\", []byte(\"yes\"), 0600); err != nil { t.Fatal(err) } }\n"
	for file, content := range map[string]string{"go.mod": "module example.com/longfixture\n\ngo 1.23\n", "fixture_test.go": source} {
		if err := os.WriteFile(filepath.Join(root, file), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	m, err := manager.New(root, config.Detect(root), runner.NewProcessRunner())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cases, err := m.Discover(ctx)
	if err != nil || len(cases) != 2 {
		t.Fatalf("discovery returned %d cases: %v", len(cases), err)
	}
	var selected domain.TestCase
	for _, tc := range cases {
		if tc.Name == name {
			selected = tc
		}
	}
	if selected.ID == "" {
		t.Fatal("the complete long test name was not discovered")
	}
	run, err := m.Run(ctx, manager.RunRequest{TestIDs: []string{selected.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	diag, err := m.Diagnostics(run.ID, 1024)
	if err != nil || diag.Run == nil || diag.Run.Status != domain.StatusPassed || diag.Run.Passed != 1 {
		t.Fatalf("targeted run: %+v, %v", diag.Run, err)
	}
	if _, err := os.Stat(filepath.Join(root, "selected")); err != nil {
		t.Fatal("selected test did not execute:", err)
	}
	if _, err := os.Stat(filepath.Join(root, "other")); !os.IsNotExist(err) {
		t.Fatalf("unselected test executed or marker could not be checked: %v", err)
	}
}
