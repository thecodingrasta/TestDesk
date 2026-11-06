package manager

import (
	"context"
	"errors"
	"testing"

	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/runner"
)

type jestConfigRunner struct {
	result runner.Result
	err    error
}

func (r jestConfigRunner) Run(_ context.Context, _ domain.CommandSpec, _ func(runner.Event)) (runner.Result, error) {
	return r.result, r.err
}

func TestJestProjectGuardRequiresUnambiguousConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result runner.Result
		err    error
		allow  bool
	}{
		{"one project", runner.Result{Stdout: `{"configs":[{}]}`}, nil, true},
		{"two projects", runner.Result{Stdout: `{"configs":[{},{}]}`}, nil, false},
		{"no projects", runner.Result{Stdout: `{"configs":[]}`}, nil, false},
		{"malformed", runner.Result{Stdout: `{`}, nil, false},
		{"command failure", runner.Result{}, errors.New("failed"), false},
		{"exit failure", runner.Result{ExitCode: 1, Stdout: `{"configs":[{}]}`}, nil, false},
		{"timeout", runner.Result{TimedOut: true, Stdout: `{"configs":[{}]}`}, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := checkJestProjectAmbiguity(context.Background(), jestConfigRunner{result: tc.result, err: tc.err}, domain.CommandSpec{Exe: "npx"})
			if (err == nil) != tc.allow {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestJestProjectGuardChecksSelectedName(t *testing.T) {
	for _, tc := range []struct {
		name, config, project string
		allow                 bool
	}{
		{"unique", `{"configs":[{"displayName":{"name":"A"}},{"displayName":{"name":"B"}}]}`, "B", true},
		{"missing", `{"configs":[{"displayName":{"name":"A"}}]}`, "B", false},
		{"duplicate", `{"configs":[{"displayName":{"name":"B"}},{"displayName":{"name":"B"}}]}`, "B", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := checkJestProjectSelection(context.Background(), jestConfigRunner{result: runner.Result{Stdout: tc.config}}, domain.CommandSpec{Exe: "npx"}, tc.project)
			if (err == nil) != tc.allow {
				t.Fatalf("got %v", err)
			}
		})
	}
}
