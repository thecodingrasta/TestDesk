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

func TestRealGoBenchmarkOutcomes(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.test/benchmark\n\ngo 1.22\n",
		"bench_test.go": `package benchmark
import "testing"
func BenchmarkPass(b *testing.B) { for i := 0; i < b.N; i++ {} }
func BenchmarkSkip(b *testing.B) { b.Skip("intentional skip") }
func BenchmarkFail(b *testing.B) { b.Fatal("intentional failure") }
func BenchmarkNested(b *testing.B) { b.Run("child", func(b *testing.B) { for i := 0; i < b.N; i++ {} }) }
func BenchmarkNestedSkip(b *testing.B) { b.Run("child", func(b *testing.B) { b.Skip("skipped child") }) }
func BenchmarkMixed(b *testing.B) { b.Run("skip", func(b *testing.B) { b.Skip("skipped child") }); b.Run("pass", func(b *testing.B) { for i := 0; i < b.N; i++ {} }) }
func TestMustNotRun(t *testing.T) { t.Fatal("ordinary tests must not run") }
`,
	}
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(root, path), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	m, err := manager.New(root, config.Detect(root), runner.NewProcessRunner())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	tests, err := m.Discover(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]domain.TestCase{}
	for _, tc := range tests {
		byName[tc.Name] = tc
	}
	for name, want := range map[string]domain.Status{"BenchmarkPass": domain.StatusPassed, "BenchmarkSkip": domain.StatusSkipped, "BenchmarkFail": domain.StatusFailed, "BenchmarkNested": domain.StatusPassed, "BenchmarkNestedSkip": domain.StatusSkipped, "BenchmarkMixed": domain.StatusPassed} {
		tc, ok := byName[name]
		if !ok {
			t.Fatalf("%s was not discovered", name)
		}
		run, err := m.Run(ctx, manager.RunRequest{TestIDs: []string{tc.ID}})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if err := m.WaitForIdle(ctx); err != nil {
			t.Fatal(err)
		}
		diag, err := m.Diagnostics(run.ID, 4096)
		if err != nil || diag.Run.Status != want {
			t.Fatalf("%s: got %+v, err %v, want %s", name, diag.Run, err, want)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "bench_test.go"), []byte(strings.ReplaceAll(files["bench_test.go"], "BenchmarkPass", "BenchmarkMoved")), 0600); err != nil {
		t.Fatal(err)
	}
	run, err := m.Run(ctx, manager.RunRequest{TestIDs: []string{byName["BenchmarkPass"].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	diag, err := m.Diagnostics(run.ID, 4096)
	if err != nil || diag.Run.Status != domain.StatusFailed {
		t.Fatalf("removed benchmark was not rejected by result evidence: %+v %v", diag.Run, err)
	}
}
