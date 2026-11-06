//go:build windows

package desktop

import "testing"

func TestCompactShowPolicyDoesNotActivateWindow(t *testing.T) {
	const noActivate = uintptr(0x0010)
	if showNoActivate != 4 {
		t.Fatalf("unexpected SW_SHOWNOACTIVATE value: %d", showNoActivate)
	}
	if noActivateFlags&noActivate == 0 {
		t.Fatalf("compact show flags must include SWP_NOACTIVATE: %#x", noActivateFlags)
	}
}

func TestWindowHandleCallbackTableStability(t *testing.T) {
	// Calling testDeskWindowHandle and nativeWindowIsMinimised thousands of times
	// must NOT exhaust the Go Windows runtime callback table (limit 2000).
	for i := 0; i < 5000; i++ {
		_ = testDeskWindowHandle()
		_ = nativeWindowIsMinimised(nil)
	}
}

func TestNativeCompactPosition(t *testing.T) {
	// First default placement (not persisted) must position at bottom-left, not top-left (0, 0)
	x, y, ok := nativeCompactPosition(0, 0, 480, 106, false)
	if !ok {
		t.Fatal("expected nativeCompactPosition to succeed")
	}
	if y <= 50 {
		t.Fatalf("expected compact widget to default to bottom of screen (y > 50), got x=%d, y=%d", x, y)
	}
	if x < 12 {
		t.Fatalf("expected compact widget margin x >= 12, got x=%d", x)
	}
}
