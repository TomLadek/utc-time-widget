//go:build windows

// utc-time-widget shows the current UTC time as a tray icon: the hour (24h)
// on top and the minute below. Its only other UI is a right-click menu with
// "Exit". It registers itself to start automatically at logon.
package main

import (
	"flag"
	"fmt"
	"image/color"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	appTitle      = "UTC Time Widget"
	className     = "utc-time-widget"
	mutexName     = `Local\utc-time-widget.single-instance`
	trayIconID    = 1
	wmTrayIcon    = wmApp + 1
	clockTimerID  = 1
	menuExitID    = 1
	fontFace      = "Segoe UI"
	fontWeight    = 700
	personalizeKP = `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`
)

// version is set at build time from the VERSION file (see build.ps1).
var version = "dev"

func main() {
	runtime.LockOSThread() // all window and GDI calls must stay on one thread

	showVersion := flag.Bool("version", false, "show the version and exit")
	removeAutostart := flag.Bool("remove-autostart", false, "remove the logon autostart entry and exit")
	flag.Parse()
	if *showVersion {
		showInfo(appTitle + " " + version)
		return
	}
	if *removeAutostart {
		if err := disableAutostart(); err != nil {
			showError("Could not remove the autostart entry:\n" + err.Error())
		}
		return
	}

	mutex, err := windows.CreateMutex(nil, false, utf16(mutexName))
	if err == mutexAlreadyExist {
		return // another instance already shows the clock
	}
	if mutex != 0 {
		defer windows.CloseHandle(mutex)
	}

	procSetProcessDpiAwarenessContext.Call(dpiPerMonitorV2)
	allowDarkMenus()

	// A failing autostart registration should not stop the clock.
	_ = enableAutostart()

	if err := run(); err != nil {
		showError(err.Error())
	}
}

type app struct {
	hwnd           windows.HWND
	taskbarCreated uint32
	renderer       *iconRenderer
	icon           windows.Handle
	iconAdded      bool
	shownKey       string // what the icon currently shows; avoids redundant redraws
}

func run() error {
	a := &app{renderer: newIconRenderer(fontFace, fontWeight)}
	defer a.renderer.close()

	r, _, _ := procRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(utf16("TaskbarCreated"))))
	a.taskbarCreated = uint32(r)

	var instance windows.Handle
	if err := windows.GetModuleHandleEx(0, nil, &instance); err != nil {
		return err
	}
	wc := wndClassEx{
		wndProc:   windows.NewCallback(a.wndProc),
		instance:  instance,
		className: utf16(className),
	}
	wc.size = uint32(unsafe.Sizeof(wc))
	if r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return fmt.Errorf("RegisterClassEx: %w", err)
	}

	// A hidden top-level window (never shown) rather than a message-only
	// window, because the latter does not receive broadcasts such as
	// TaskbarCreated or WM_SETTINGCHANGE.
	hwnd, _, err := procCreateWindowExW.Call(0,
		uintptr(unsafe.Pointer(utf16(className))),
		uintptr(unsafe.Pointer(utf16(appTitle))),
		0, 0, 0, 0, 0, 0, 0, uintptr(instance), 0)
	if hwnd == 0 {
		return fmt.Errorf("CreateWindowEx: %w", err)
	}
	a.hwnd = windows.HWND(hwnd)

	a.refresh(true)
	a.scheduleTick()

	var m msg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return nil
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func (a *app) wndProc(hwnd windows.HWND, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case wmTrayIcon:
		// NOTIFYICON_VERSION_4: event in LOWORD(lParam), anchor point in wParam.
		if uint32(lParam&0xFFFF) == wmContextMenu {
			a.showMenu(int16(wParam&0xFFFF), int16((wParam>>16)&0xFFFF))
		}
		return 0
	case wmTimer:
		a.refresh(false)
		a.scheduleTick()
		return 0
	case wmSettingChange, wmDisplayChange, wmTimeChange:
		a.refresh(true)
	case wmPowerBroadcast:
		if wParam == pbtAPMResumeAuto || wParam == pbtAPMResumeSusp {
			a.refresh(true)
		}
	case wmDestroy:
		a.removeTrayIcon()
		procPostQuitMessage.Call(0)
		return 0
	default:
		if message == a.taskbarCreated && message != 0 {
			// Explorer (re)started: our icon is gone and must be added again.
			a.iconAdded = false
			a.refresh(true)
			return 0
		}
	}
	r, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return r
}

// scheduleTick fires the timer at the next minute boundary, but at least once
// a second so theme, DPI and clock changes are picked up promptly.
func (a *app) scheduleTick() {
	now := time.Now()
	untilNextMinute := now.Truncate(time.Minute).Add(time.Minute).Sub(now) + 20*time.Millisecond
	delay := min(untilNextMinute, time.Second)
	procSetTimer.Call(uintptr(a.hwnd), clockTimerID, uintptr(delay.Milliseconds()), 0)
}

// refresh redraws the icon if the time, icon size or taskbar theme changed.
func (a *app) refresh(force bool) {
	now := time.Now().UTC()
	size := trayIconSize()
	light := taskbarUsesLightTheme()
	key := fmt.Sprintf("%s|%d|%t", now.Format("15:04"), size, light)
	if !force && a.iconAdded && key == a.shownKey {
		return
	}

	fg := color.NRGBA{255, 255, 255, 255}
	if light {
		fg = color.NRGBA{0, 0, 0, 255}
	}
	img, err := a.renderer.render(size, now.Format("15"), now.Format("04"), fg)
	if err != nil {
		return
	}
	icon, err := createIcon(img)
	if err != nil {
		return
	}
	tip := "UTC " + now.Format("15:04") + "\n" + now.Format("Mon, 02 Jan 2006")

	if a.updateTrayIcon(icon, tip) {
		destroyIcon(a.icon)
		a.icon = icon
		a.shownKey = key
	} else {
		destroyIcon(icon) // explorer not ready yet; retried on the next tick
	}
}

func (a *app) notifyData() notifyIconData {
	nid := notifyIconData{hwnd: a.hwnd, id: trayIconID}
	nid.size = uint32(unsafe.Sizeof(nid))
	return nid
}

func (a *app) updateTrayIcon(icon windows.Handle, tip string) bool {
	nid := a.notifyData()
	nid.flags = nifMessage | nifIcon | nifTip | nifShowTip
	nid.callbackMessage = wmTrayIcon
	nid.icon = icon
	tip16, _ := windows.UTF16FromString(tip)
	copy(nid.tip[:maxTooltipLength-1], tip16)

	if a.iconAdded && shellNotifyIcon(nimModify, &nid) {
		return true
	}
	// Not added yet, or explorer lost it: (re)add it.
	shellNotifyIcon(nimDelete, &nid)
	if !shellNotifyIcon(nimAdd, &nid) {
		a.iconAdded = false
		return false
	}
	nid.version = notifyIconVersion
	shellNotifyIcon(nimSetVersion, &nid)
	a.iconAdded = true
	return true
}

func (a *app) removeTrayIcon() {
	nid := a.notifyData()
	shellNotifyIcon(nimDelete, &nid)
	a.iconAdded = false
	destroyIcon(a.icon)
	a.icon = 0
}

func shellNotifyIcon(op uint32, nid *notifyIconData) bool {
	r, _, _ := procShellNotifyIconW.Call(uintptr(op), uintptr(unsafe.Pointer(nid)))
	return r != 0
}

func (a *app) showMenu(x, y int16) {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)
	procAppendMenuW.Call(menu, mfString, menuExitID, uintptr(unsafe.Pointer(utf16("Exit"))))

	flags := uintptr(tpmRightButton | tpmBottomAlign | tpmReturnCmd | tpmNoNotify)
	if r, _, _ := procGetSystemMetrics.Call(smMenuDropAlign); r != 0 {
		flags |= tpmRightAlign
	}
	// Required so the menu closes when the user clicks elsewhere.
	procSetForegroundWindow.Call(uintptr(a.hwnd))
	cmd, _, _ := procTrackPopupMenu.Call(menu, flags, uintptr(x), uintptr(y), 0, uintptr(a.hwnd), 0)
	procPostMessageW.Call(uintptr(a.hwnd), wmNull, 0, 0)

	if cmd == menuExitID {
		procDestroyWindow.Call(uintptr(a.hwnd))
	}
}

// trayIconSize returns the small-icon size for the DPI of the taskbar, which
// may differ from the system DPI the process started with.
func trayIconSize() int {
	dpi := uintptr(0)
	if tray, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(utf16("Shell_TrayWnd"))), 0); tray != 0 {
		dpi, _, _ = procGetDpiForWindow.Call(tray)
	}
	if dpi == 0 {
		dpi, _, _ = procGetDpiForSystem.Call()
	}
	if dpi == 0 {
		dpi = fallbackDPI
	}
	size, _, _ := procGetSystemMetricsForDpi.Call(smCxSmIcon, dpi)
	if int32(size) <= 0 {
		return fallbackIconSize
	}
	return int(int32(size))
}

// taskbarUsesLightTheme reports whether the taskbar is light ("Choose your
// Windows mode" = Light), in which case dark digits are needed.
func taskbarUsesLightTheme() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, personalizeKP, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("SystemUsesLightTheme")
	return err == nil && v != 0
}

// allowDarkMenus lets the context menu follow the Windows dark mode. It uses
// the undocumented (but long stable) uxtheme SetPreferredAppMode export, and
// silently does nothing if it is unavailable.
func allowDarkMenus() {
	h, err := windows.LoadLibraryEx("uxtheme.dll", 0, windows.LOAD_LIBRARY_SEARCH_SYSTEM32)
	if err != nil {
		return
	}
	const setPreferredAppMode, flushMenuThemes, allowDark = 135, 136, 1
	if p, err := windows.GetProcAddressByOrdinal(h, setPreferredAppMode); err == nil {
		syscall.SyscallN(p, allowDark)
	}
	if p, err := windows.GetProcAddressByOrdinal(h, flushMenuThemes); err == nil {
		syscall.SyscallN(p)
	}
}
