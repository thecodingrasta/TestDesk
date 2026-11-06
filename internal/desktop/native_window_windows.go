//go:build windows

package desktop

import (
	"context"
	"os"
	"sync"
	"syscall"
	"unsafe"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	windowStyleIndex  = ^uintptr(15) // GWL_STYLE (-16)
	windowFrameMask   = uintptr(0x00CF0000)
	frameChangedFlags = uintptr(0x0027) // NOMOVE | NOSIZE | NOZORDER | FRAMECHANGED
	roundedCorner     = uint32(2)
	showNoActivate    = uintptr(4)
	noActivateFlags   = uintptr(0x0057) // NOSIZE | NOMOVE | NOZORDER | NOACTIVATE | SHOWWINDOW
)

var (
	user32DLL                = syscall.NewLazyDLL("user32.dll")
	dwmapiDLL                = syscall.NewLazyDLL("dwmapi.dll")
	enumWindows              = user32DLL.NewProc("EnumWindows")
	getWindowThreadProcessID = user32DLL.NewProc("GetWindowThreadProcessId")
	getWindowTextW           = user32DLL.NewProc("GetWindowTextW")
	isWindow                 = user32DLL.NewProc("IsWindow")
	isIconic                 = user32DLL.NewProc("IsIconic")
	getWindowLongPtrW        = user32DLL.NewProc("GetWindowLongPtrW")
	setWindowLongPtrW        = user32DLL.NewProc("SetWindowLongPtrW")
	setWindowPos             = user32DLL.NewProc("SetWindowPos")
	showWindow               = user32DLL.NewProc("ShowWindow")
	monitorFromWindow        = user32DLL.NewProc("MonitorFromWindow")
	getMonitorInfoW          = user32DLL.NewProc("GetMonitorInfoW")
	dwmSetWindowAttribute    = dwmapiDLL.NewProc("DwmSetWindowAttribute")

	cachedHwnd   uintptr
	cachedHwndMu sync.Mutex
	enumCallback uintptr
	enumTarget   uintptr
)

func init() {
	enumCallback = syscall.NewCallback(func(hwnd uintptr, lparam uintptr) uintptr {
		var processID uint32
		getWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&processID)))
		if processID == uint32(os.Getpid()) {
			var title [256]uint16
			getWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&title[0])), uintptr(len(title)))
			if syscall.UTF16ToString(title[:]) == "TestDesk" {
				enumTarget = hwnd
				return 0
			}
		}
		return 1
	})
}

func testDeskWindowHandle() uintptr {
	cachedHwndMu.Lock()
	defer cachedHwndMu.Unlock()
	if cachedHwnd != 0 {
		if ok, _, _ := isWindow.Call(cachedHwnd); ok != 0 {
			return cachedHwnd
		}
		cachedHwnd = 0
	}
	enumTarget = 0
	enumWindows.Call(enumCallback, 0)
	if enumTarget != 0 {
		cachedHwnd = enumTarget
		return cachedHwnd
	}
	return 0
}

func nativeWindowIsMinimised(context.Context) bool {
	hwnd := testDeskWindowHandle()
	if hwnd == 0 {
		return false
	}
	minimised, _, _ := isIconic.Call(hwnd)
	return minimised != 0
}

func nativeSetCompactFrame(_ context.Context, compact bool, width int, height int) {
	hwnd := testDeskWindowHandle()
	if hwnd == 0 {
		return
	}
	style, _, _ := getWindowLongPtrW.Call(hwnd, windowStyleIndex)
	if style != 0 {
		if compact {
			style &^= windowFrameMask
		} else {
			style |= windowFrameMask
		}
		setWindowLongPtrW.Call(hwnd, windowStyleIndex, style)
		flags := frameChangedFlags
		if width > 0 && height > 0 {
			flags &^= uintptr(0x0001) // remove SWP_NOSIZE
			setWindowPos.Call(hwnd, 0, 0, 0, uintptr(width), uintptr(height), flags)
		} else {
			setWindowPos.Call(hwnd, 0, 0, 0, 0, 0, flags)
		}
	}
	corner := roundedCorner
	dwmSetWindowAttribute.Call(hwnd, 33, uintptr(unsafe.Pointer(&corner)), unsafe.Sizeof(corner))
}

func nativeShowCompact(ctx context.Context, activate bool) {
	if activate {
		runtime.WindowShow(ctx)
		return
	}
	hwnd := testDeskWindowHandle()
	if hwnd == 0 {
		runtime.WindowShow(ctx)
		return
	}
	showWindow.Call(hwnd, showNoActivate)
	setWindowPos.Call(hwnd, 0, 0, 0, 0, 0, noActivateFlags)
}

type nativeRect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type nativeMonitorInfo struct {
	Size    uint32
	Monitor nativeRect
	Work    nativeRect
	Flags   uint32
}

func nativeCompactPosition(x int, y int, width int, height int, persisted bool) (int, int, bool) {
	hwnd := testDeskWindowHandle()
	monitor, _, _ := monitorFromWindow.Call(hwnd, 1) // 1 = MONITOR_DEFAULTTOPRIMARY
	if monitor == 0 {
		return 16, 700, true
	}
	info := nativeMonitorInfo{Size: uint32(unsafe.Sizeof(nativeMonitorInfo{}))}
	ok, _, _ := getMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		return 16, 700, true
	}
	margin := int32(12)
	left := info.Work.Left + margin
	top := info.Work.Top + margin
	right := info.Work.Right - int32(width) - margin
	bottom := info.Work.Bottom - int32(height) - margin
	if right < left {
		right = left
	}
	if bottom < top {
		bottom = top
	}
	if !persisted || x == 0 && y == 0 {
		return int(left), int(bottom), true
	}
	positionX := int32(x)
	positionY := int32(y)
	if positionX < left {
		positionX = left
	} else if positionX > right {
		positionX = right
	}
	if positionY < top {
		positionY = top
	} else if positionY > bottom {
		positionY = bottom
	}
	return int(positionX), int(positionY), true
}
