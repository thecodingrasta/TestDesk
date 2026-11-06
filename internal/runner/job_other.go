//go:build !windows

package runner

import (
	"context"

	"github.com/thecodingrasta/testdesk/internal/domain"
)

func runPlatformProcess(context.Context, domain.CommandSpec, func(Event)) (Result, error, bool) {
	return Result{}, nil, false
}
