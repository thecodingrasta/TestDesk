//go:build !windows

package desktop

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func nativeWindowIsMinimised(_ context.Context) bool {
	return false
}

func nativeSetCompactFrame(context.Context, bool, int, int) {}

func nativeShowCompact(ctx context.Context, _ bool) {
	runtime.WindowShow(ctx)
}

func nativeCompactPosition(int, int, int, int, bool) (int, int, bool) {
	return 0, 0, false
}
