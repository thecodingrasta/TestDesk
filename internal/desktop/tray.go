//go:build !darwin

package desktop

import (
	"errors"
	"runtime"
	"sync"

	"github.com/getlantern/systray"
)

// trayService owns a dedicated, locked native message loop. Wails only pumps
// its own window thread; relying on systray.Register left the icon visible but
// its menu window unable to process clicks on Windows.
type trayService struct {
	app      *App
	ready    chan struct{}
	stopOnce sync.Once
}

func startNativeTray(app *App) (*trayService, error) {
	if app == nil {
		return nil, errors.New("tray requires an app")
	}
	tray := &trayService{app: app, ready: make(chan struct{})}
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		systray.Run(tray.onReady, func() {})
	}()
	return tray, nil
}

func (t *trayService) available() bool {
	select {
	case <-t.ready:
		return true
	default:
		return false
	}
}

func (t *trayService) stop() {
	t.stopOnce.Do(func() { systray.Quit() })
}

func (t *trayService) onReady() {
	if icon, err := assets.ReadFile("assets/tray.ico"); err == nil {
		systray.SetIcon(icon)
	}
	systray.SetTooltip("TestDesk - test lifecycle manager")

	open := systray.AddMenuItem("Open Test Manager", "Show the test manager")
	controller := systray.AddMenuItem("Show Controller", "Show the compact controller")
	projects := systray.AddMenuItem("Projects / Add Project", "Manage test projects")
	refresh := systray.AddMenuItem("Refresh Tests", "Discover tests for the active project")
	systray.AddSeparator()
	pause := systray.AddMenuItem("Pause Queue", "Pause queued test execution")
	resume := systray.AddMenuItem("Resume Queue", "Resume queued test execution")
	systray.AddSeparator()
	quit := systray.AddMenuItem("Exit TestDesk", "Quit TestDesk completely")

	go func() {
		for range open.ClickedCh {
			_ = t.app.SetWindowMode("manager")
		}
	}()
	go func() {
		for range controller.ClickedCh {
			_ = t.app.SetWindowMode("compact")
			_ = t.app.ApplyWidgetVisibility(true)
		}
	}()
	go func() {
		for range projects.ClickedCh {
			_ = t.app.SetWindowMode("setup")
		}
	}()
	go func() {
		for range refresh.ClickedCh {
			_, _ = t.app.Discover(false)
		}
	}()
	go func() {
		for range pause.ClickedCh {
			_ = t.app.Control("pause")
		}
	}()
	go func() {
		for range resume.ClickedCh {
			_ = t.app.Control("resume")
		}
	}()
	go func() {
		for range quit.ClickedCh {
			t.app.Quit()
		}
	}()

	close(t.ready)
}
