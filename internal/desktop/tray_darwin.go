//go:build darwin

package desktop

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa
#include "tray_darwin.h"
*/
import "C"

import (
	"errors"
	"sync"
	"sync/atomic"
	"unsafe"
)

// Wails owns NSApplication and its event loop. The native tray adds a status
// item to that existing loop without replacing the application delegate.
type trayService struct {
	app      *App
	ready    chan struct{}
	stopOnce sync.Once
	stopped  atomic.Bool
}

var darwinTray struct {
	sync.Mutex
	service *trayService
}

func startNativeTray(app *App) (*trayService, error) {
	if app == nil {
		return nil, errors.New("tray requires an app")
	}
	icon, err := assets.ReadFile("assets/testdesk-brand-icon.png")
	if err != nil {
		return nil, err
	}
	if len(icon) == 0 {
		return nil, errors.New("tray icon is empty")
	}
	tray := &trayService{app: app, ready: make(chan struct{})}
	darwinTray.Lock()
	darwinTray.service = tray
	darwinTray.Unlock()
	C.testdeskTrayStart(unsafe.Pointer(&icon[0]), C.int(len(icon)))
	return tray, nil
}

func (t *trayService) available() bool {
	if t.stopped.Load() {
		return false
	}
	select {
	case <-t.ready:
		return true
	default:
		return false
	}
}

func (t *trayService) stop() {
	t.stopOnce.Do(func() {
		t.stopped.Store(true)
		darwinTray.Lock()
		if darwinTray.service == t {
			darwinTray.service = nil
		}
		darwinTray.Unlock()
		C.testdeskTrayStop()
	})
}

//export testdeskTrayReady
func testdeskTrayReady() {
	darwinTray.Lock()
	t := darwinTray.service
	if t != nil && !t.stopped.Load() {
		close(t.ready)
	}
	darwinTray.Unlock()
}

//export testdeskTrayMenuSelected
func testdeskTrayMenuSelected(command C.int) {
	darwinTray.Lock()
	t := darwinTray.service
	darwinTray.Unlock()
	if t == nil || t.stopped.Load() {
		return
	}
	// AppKit invokes the callback on its main queue. Wails window operations may
	// dispatch to that queue too, so execute them after returning to AppKit.
	go t.handleCommand(int(command))
}

func (t *trayService) handleCommand(command int) {
	if t.stopped.Load() {
		return
	}
	switch command {
	case 1:
		_ = t.app.SetWindowMode("manager")
	case 2:
		_ = t.app.SetWindowMode("compact")
		_ = t.app.ApplyWidgetVisibility(true)
	case 3:
		_ = t.app.SetWindowMode("setup")
	case 4:
		_, _ = t.app.Discover(false)
	case 5:
		_ = t.app.Control("pause")
	case 6:
		_ = t.app.Control("resume")
	case 7:
		t.app.Quit()
	}
}
