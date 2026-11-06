package manager_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
)

func TestDiagnosticsRejectsRunPathTraversal(t *testing.T) {
	project := copySampleProject(t)
	mgr, err := manager.NewFromProject(project)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(mgr.StoreRoot(), "sibling")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "output.log"), []byte("private sibling log\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "diagnostics.json"), []byte(`{"id":"sibling","status":"failed"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, runID := range []string{"../sibling", `..\sibling`, " sibling ", "C:sibling", ".", ".."} {
		if diag, err := mgr.Diagnostics(runID, 4096); err == nil {
			t.Errorf("run ID %q returned %+v", runID, diag)
		}
	}
	const validID = "valid-run"
	if err := mgr.Store().AppendLog(validID, "valid log"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Store().SaveRun(domain.Run{ID: validID, Status: domain.StatusPassed}); err != nil {
		t.Fatal(err)
	}
	diag, err := mgr.Diagnostics(validID, 4096)
	if err != nil || diag.Run == nil || diag.Run.ID != validID || !strings.Contains(diag.Log, "valid log") {
		t.Fatalf("valid diagnostics: %+v, %v", diag, err)
	}
}
