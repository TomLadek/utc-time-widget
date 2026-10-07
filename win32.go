//go:build windows

package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// Minimal Win32 bindings. Only system DLLs are loaded (from System32), so the
// resulting executable has no dependencies beyond a stock Windows install.

var (
	user32  = windows.NewLazySystemDLL("user32.dll")
	gdi32   = windows.NewLazySystemDLL("gdi32.dll")
	shell32 = windows.NewLazySystemDLL("shell32.dll")

	procAppendMenuW                   = user32.NewProc("AppendMenuW")
	procCreateIconIndirect            = user32.NewProc("CreateIconIndirect")
	procCreatePopupMenu               = user32.NewProc("CreatePopupMenu")
	procCreateWindowExW               = user32.NewProc("CreateWindowExW")
	procDefWindowProcW                = user32.NewProc("DefWindowProcW")
	procDestroyIcon                   = user32.NewProc("DestroyIcon")
	procDestroyMenu                   = user32.NewProc("DestroyMenu")
	procDestroyWindow                 = user32.NewProc("DestroyWindow")
	procDispatchMessageW              = user32.NewProc("DispatchMessageW")
	procFindWindowW                   = user32.NewProc("FindWindowW")
	procGetDpiForSystem               = user32.NewProc("GetDpiForSystem")
	procGetDpiForWindow               = user32.NewProc("GetDpiForWindow")
	procGetMessageW                   = user32.NewProc("GetMessageW")
	procGetSystemMetrics              = user32.NewProc("GetSystemMetrics")
	procGetSystemMetricsForDpi        = user32.NewProc("GetSystemMetricsForDpi")
	procMessageBoxW                   = user32.NewProc("MessageBoxW")
	procPostMessageW                  = user32.NewProc("PostMessageW")
	procPostQuitMessage               = user32.NewProc("PostQuitMessage")
	procRegisterClassExW              = user32.NewProc("RegisterClassExW")
	procRegisterWindowMessageW        = user32.NewProc("RegisterWindowMessageW")
	procSetForegroundWindow           = user32.NewProc("SetForegroundWindow")
	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procSetTimer                      = user32.NewProc("SetTimer")
	procTrackPopupMenu                = user32.NewProc("TrackPopupMenu")
	procTranslateMessage              = user32.NewProc("TranslateMessage")

	procCreateBitmap       = gdi32.NewProc("CreateBitmap")
	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procCreateFontW        = gdi32.NewProc("CreateFontW")
	procDeleteDC           = gdi32.NewProc("DeleteDC")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
	procGdiFlush           = gdi32.NewProc("GdiFlush")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procSetBkMode          = gdi32.NewProc("SetBkMode")
	procSetTextColor       = gdi32.NewProc("SetTextColor")
	procTextOutW           = gdi32.NewProc("TextOutW")

	procShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")
)

const (
	wmNull            = 0x0000
	wmDestroy         = 0x0002
	wmSettingChange   = 0x001A
	wmTimeChange      = 0x001E
	wmContextMenu     = 0x007B
	wmDisplayChange   = 0x007E
	wmTimer           = 0x0113
	wmPowerBroadcast  = 0x0218
	wmApp             = 0x8000
	pbtAPMResumeAuto  = 0x0012
	pbtAPMResumeSusp  = 0x0007
	smCxSmIcon        = 49
	smMenuDropAlign   = 40
	mfString          = 0x0000
	tpmRightButton    = 0x0002
	tpmRightAlign     = 0x0008
	tpmBottomAlign    = 0x0020
	tpmNoNotify       = 0x0080
	tpmReturnCmd      = 0x0100
	mbIconError       = 0x0010
	mbIconInformation = 0x0040
	nimAdd            = 0x0
	nimModify         = 0x1
	nimDelete         = 0x2
	nimSetVersion     = 0x4
	nifMessage        = 0x01
	nifIcon           = 0x02
	nifTip            = 0x04
	nifShowTip        = 0x80
	notifyIconVersion = 4

	biRGB             = 0
	dibRGBColors      = 0
	bkTransparent     = 1
	fontCharsetDef    = 1
	fontOutTTPrecis   = 4
	fontAntialiased   = 4
	dpiPerMonitorV2   = ^uintptr(3) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 (-4)
	fallbackDPI       = 96
	fallbackIconSize  = 16
	maxTooltipLength  = 128
	mutexAlreadyExist = windows.ERROR_ALREADY_EXISTS
)

type point struct{ x, y int32 }

type msg struct {
	hwnd     windows.HWND
	message  uint32
	wParam   uintptr
	lParam   uintptr
	time     uint32
	pt       point
	lPrivate uint32
}

type wndClassEx struct {
	size       uint32
	style      uint32
	wndProc    uintptr
	clsExtra   int32
	wndExtra   int32
	instance   windows.Handle
	icon       windows.Handle
	cursor     windows.Handle
	background windows.Handle
	menuName   *uint16
	className  *uint16
	iconSm     windows.Handle
}

type notifyIconData struct {
	size            uint32
	hwnd            windows.HWND
	id              uint32
	flags           uint32
	callbackMessage uint32
	icon            windows.Handle
	tip             [maxTooltipLength]uint16
	state           uint32
	stateMask       uint32
	info            [256]uint16
	version         uint32 // union with uTimeout
	infoTitle       [64]uint16
	infoFlags       uint32
	guidItem        windows.GUID
	balloonIcon     windows.Handle
}

type iconInfo struct {
	isIcon   int32
	xHotspot uint32
	yHotspot uint32
	mask     windows.Handle
	color    windows.Handle
}

type bitmapInfo struct {
	size          uint32
	width         int32
	height        int32
	planes        uint16
	bitCount      uint16
	compression   uint32
	sizeImage     uint32
	xPelsPerMeter int32
	yPelsPerMeter int32
	clrUsed       uint32
	clrImportant  uint32
	colors        [1]uint32
}

func utf16(s string) *uint16 {
	p, err := windows.UTF16PtrFromString(s)
	if err != nil {
		panic(err)
	}
	return p
}

func deleteObject(h windows.Handle) {
	if h != 0 {
		procDeleteObject.Call(uintptr(h))
	}
}

func destroyIcon(h windows.Handle) {
	if h != 0 {
		procDestroyIcon.Call(uintptr(h))
	}
}

// createDIB creates a top-down 32-bit BGRA DIB section and returns its pixels.
func createDIB(w, h int) (windows.Handle, []byte, error) {
	bi := bitmapInfo{
		width:       int32(w),
		height:      -int32(h), // negative height = top-down rows
		planes:      1,
		bitCount:    32,
		compression: biRGB,
	}
	bi.size = uint32(unsafe.Offsetof(bi.colors))
	var bits unsafe.Pointer
	r, _, err := procCreateDIBSection.Call(0, uintptr(unsafe.Pointer(&bi)), dibRGBColors,
		uintptr(unsafe.Pointer(&bits)), 0, 0)
	if r == 0 || bits == nil {
		return 0, nil, err
	}
	return windows.Handle(r), unsafe.Slice((*byte)(bits), w*h*4), nil
}

func showError(text string) { messageBox(text, mbIconError) }

func showInfo(text string) { messageBox(text, mbIconInformation) }

func messageBox(text string, flags uintptr) {
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(utf16(text))),
		uintptr(unsafe.Pointer(utf16(appTitle))), flags)
}
