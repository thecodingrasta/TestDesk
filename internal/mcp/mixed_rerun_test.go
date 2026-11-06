package mcp

import (
	"context"
	"fmt"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
	"github.com/thecodingrasta/testdesk/internal/runner"
	"testing"
	"time"
)

type mixedRerunRunner struct{}

func (mixedRerunRunner) Run(_ context.Context, cmd domain.CommandSpec, _ func(runner.Event)) (runner.Result, error) {
	if cmd.Exe == "discover" {
		return runner.Result{Stdout: "fixture-case\n"}, nil
	}
	return runner.Result{ExitCode: 1, Stderr: "intentional failure"}, fmt.Errorf("intentional failure")
}

func TestRerunFailedPreservesMixedScopes(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	providers := []domain.Provider{
		{ID: "individual", Name: "individual", Kind: domain.ProviderCustom, Enabled: true, Discover: domain.DiscoverSpec{Parser: "lines", Command: domain.CommandSpec{Exe: "discover"}}, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "all-one"}, Test: domain.CommandSpec{Exe: "single"}}},
		{ID: "provider", Name: "provider", Kind: domain.ProviderCustom, Enabled: true, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "all-two"}}},
	}
	mgr, err := manager.New(t.TempDir(), domain.ProjectConfig{Name: "fixture", Providers: providers}, mixedRerunRunner{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := mgr.Discover(ctx); err != nil {
		t.Fatal(err)
	}
	first, err := mgr.Run(ctx, manager.RunRequest{ProviderIDs: []string{"individual", "provider"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	diag, err := mgr.Diagnostics(first.ID, 1024)
	if err != nil || len(diag.Run.Items) != 2 || diag.Run.Failed != 2 {
		t.Fatalf("initial run: %+v %v", diag.Run, err)
	}
	if _, err := New(mgr).rerunFailed(ctx, map[string]any{"runId": "missing-run"}); err == nil {
		t.Fatal("missing run ID unexpectedly reran another run")
	}
	if _, err := New(mgr).rerunFailed(ctx, map[string]any{"runId": "../" + first.ID}); err == nil {
		t.Fatal("path-like run ID unexpectedly selected another run")
	}
	result, err := New(mgr).rerunFailed(ctx, map[string]any{"runId": first.ID})
	if err != nil {
		t.Fatal(err)
	}
	payload := result.(map[string]any)
	if payload["total"] != 2 {
		t.Fatalf("rerun payload: %+v", payload)
	}
	if err := mgr.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	rerun, err := mgr.Diagnostics(payload["runId"].(string), 1024)
	if err != nil || len(rerun.Run.Items) != 2 {
		t.Fatalf("rerun: %+v %v", rerun.Run, err)
	}
	found := map[string]domain.RunItem{}
	for _, item := range rerun.Run.Items {
		found[item.ProviderID] = item
	}
	if found["individual"].TestID == "" || found["provider"].TestID != "" || found["provider"].CommandPreview != "all-two" {
		t.Fatalf("rerun widened or lost scope: %+v", rerun.Run.Items)
	}
}
