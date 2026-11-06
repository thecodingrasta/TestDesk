package manager_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
	"github.com/thecodingrasta/testdesk/internal/runner"
)

func TestIncompleteGoDiscoveryRetainsPreviousCatalog(t *testing.T) {
	root := t.TempDir()
	first := `{"Action":"start","Package":"example/first"}` + "\n" + `{"Action":"output","Package":"example/first","Output":"TestFirst\n"}` + "\n" + `{"Action":"pass","Package":"example/first"}`
	second := `{"Action":"start","Package":"example/second"}` + "\n" + `{"Action":"output","Package":"example/second","Output":"TestSecond\n"}` + "\n" + `{"Action":"pass","Package":"example/second"}`
	proc := &discoveryFailureRunner{result: runner.Result{Stdout: first + "\n" + second}}
	cfg := domain.ProjectConfig{Version: 1, Name: "fixture", Providers: []domain.Provider{config.GoTestProvider()}}
	m, err := manager.New(root, cfg, proc)
	if err != nil {
		t.Fatal(err)
	}
	if cases, err := m.Discover(context.Background()); err != nil || len(cases) != 2 {
		t.Fatalf("initial: %v, %v", cases, err)
	}
	proc.result.Stdout = first + "\n" + `{"Action":"start","Package":"example/second"}`
	if cases, err := m.Discover(context.Background()); err == nil || len(cases) != 2 {
		t.Fatalf("partial: %v, %v", cases, err)
	}
	reopened, err := manager.New(root, cfg, proc)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := reopened.Snapshot(domain.Filter{Limit: 10})
	if err != nil || len(snapshot.Tests) != 2 {
		t.Fatalf("reopened: %v, %v", snapshot.Tests, err)
	}
}

func TestRealGoDiscoveryAcceptsPackageWithoutTests(t *testing.T) {
	root := t.TempDir()
	for name, contents := range map[string]string{"go.mod": "module example.com/empty\n\ngo 1.23\n", "empty.go": "package empty\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	m, err := manager.New(root, config.Detect(root), runner.NewProcessRunner())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cases, err := m.Discover(ctx)
	if err != nil || len(cases) != 0 {
		t.Fatalf("package with no tests: %v, %v", cases, err)
	}
}
