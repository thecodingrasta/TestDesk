package manager_test

import (
	"context"
	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
	"github.com/thecodingrasta/testdesk/internal/runner"
	"testing"
)

type cargoDiscoveryRunner struct{ incomplete, incompleteListing bool }

func (r *cargoDiscoveryRunner) Run(_ context.Context, command domain.CommandSpec, _ func(runner.Event)) (runner.Result, error) {
	if len(command.Args) > 1 && command.Args[1] == "--package" {
		if r.incompleteListing {
			return runner.Result{Stdout: "module::known: test\n\n2 tests, 0 benchmarks\n"}, nil
		}
		return runner.Result{Stdout: "module::known: test\n\n1 test, 0 benchmarks\n"}, nil
	}
	build := `{"reason":"compiler-artifact","package_id":"fixture@1","executable":"fixture-binary","target":{"name":"fixture","kind":["lib"]},"profile":{"test":true}}` + "\n"
	if !r.incomplete {
		build += `{"reason":"build-finished","success":true}` + "\n"
	}
	return runner.Result{Stdout: build}, nil
}

func TestCargoIncompleteDiscoveryRetainsPriorCatalog(t *testing.T) {
	root := t.TempDir()
	proc := &cargoDiscoveryRunner{}
	cfg := domain.ProjectConfig{Version: 1, Name: "fixture", Providers: []domain.Provider{config.CargoProvider()}}
	m, err := manager.New(root, cfg, proc)
	if err != nil {
		t.Fatal(err)
	}
	before, err := m.Discover(context.Background())
	if err != nil || len(before) != 1 {
		t.Fatalf("initial catalog: %+v %v", before, err)
	}
	proc.incomplete = true
	after, err := m.Discover(context.Background())
	if err == nil || len(after) != 1 || before[0].ID != after[0].ID {
		t.Fatalf("partial build replaced catalog: %+v %v", after, err)
	}
	proc.incomplete = false
	proc.incompleteListing = true
	after, err = m.Discover(context.Background())
	if err == nil || len(after) != 1 || before[0].ID != after[0].ID {
		t.Fatalf("partial listing replaced catalog: %+v %v", after, err)
	}
}
