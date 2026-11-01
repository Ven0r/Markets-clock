//go:build windows

package main

// Minimal hand-written Win32 bindings (standard library only, no cgo).

import (
	"syscall"
	"unsafe"
)

// System DLLs are loaded by absolute path from System32 so a stray DLL next
// to the exe can never be picked up instead.
var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll") // always a KnownDLL
	sysDir   = systemDir()

	user32   = sysDLL("user32.dll")
	gdi32    = sysDLL("gdi32.dll")
	shell32  = sysDLL("shell32.dll")
	dwmapi   = sysDLL("dwmapi.dll")
	advapi32 = sysDLL("advapi32.dll")
	uxtheme  = sysDLL("uxtheme.dll")

	pGetSystemDirectoryW           = kernel32.NewProc("GetSystemDirectoryW")
	pGetModuleHandleW              = kernel32.NewProc("GetModuleHandleW")
	pCreateMutexW                  = kernel32.NewProc("CreateMutexW")
	pGetDynamicTimeZoneInformation = kernel32.NewProc("GetDynamicTimeZoneInformation")
	pGetProcAddress                = kernel32.NewProc("GetProcAddress")

	pCreateEventW                = kernel32.NewProc("CreateEventW")
	pGetCurrentProcessId         = kernel32.NewProc("GetCurrentProcessId")
	pProcessIdToSessionId        = kernel32.NewProc("ProcessIdToSessionId")
	pMsgWaitForMultipleObjectsEx = user32.NewProc("MsgWaitForMultipleObjectsEx")
	pPeekMessageW                = user32.NewProc("PeekMessageW")
	pRegNotifyChangeKeyValue     = advapi32.NewProc("RegNotifyChangeKeyValue")

	pRegisterClassExW              = user32.NewProc("RegisterClassExW")
	pCreateWindowExW               = user32.NewProc("CreateWindowExW")
	pDefWindowProcW                = user32.NewProc("DefWindowProcW")
	pDestroyWindow                 = user32.NewProc("DestroyWindow")
	pGetMessageW                   = user32.NewProc("GetMessageW")
	pTranslateMessage              = user32.NewProc("TranslateMessage")
	pDispatchMessageW              = user32.NewProc("DispatchMessageW")
	pPostQuitMessage               = user32.NewProc("PostQuitMessage")
	pShowWindow                    = user32.NewProc("ShowWindow")
	pIsWindowVisible               = user32.NewProc("IsWindowVisible")
	pInvalidateRect                = user32.NewProc("InvalidateRect")
	pBeginPaint                    = user32.NewProc("BeginPaint")
	pEndPaint                      = user32.NewProc("EndPaint")
	pGetClientRect                 = user32.NewProc("GetClientRect")
	pGetWindowRect                 = user32.NewProc("GetWindowRect")
	pSetWindowPos                  = user32.NewProc("SetWindowPos")
	pSetTimer                      = user32.NewProc("SetTimer")
	pKillTimer                     = user32.NewProc("KillTimer")
	pLoadCursorW                   = user32.NewProc("LoadCursorW")
	pLoadIconW                     = user32.NewProc("LoadIconW")
	pLoadImageW                    = user32.NewProc("LoadImageW")
	pCreatePopupMenu               = user32.NewProc("CreatePopupMenu")
	pAppendMenuW                   = user32.NewProc("AppendMenuW")
	pCheckMenuRadioItem            = user32.NewProc("CheckMenuRadioItem")
	pTrackPopupMenu                = user32.NewProc("TrackPopupMenu")
	pDestroyMenu                   = user32.NewProc("DestroyMenu")
	pSetForegroundWindow           = user32.NewProc("SetForegroundWindow")
	pGetCursorPos                  = user32.NewProc("GetCursorPos")
	pReleaseCapture                = user32.NewProc("ReleaseCapture")
	pSendMessageW                  = user32.NewProc("SendMessageW")
	pPostMessageW                  = user32.NewProc("PostMessageW")
	pTrackMouseEvent               = user32.NewProc("TrackMouseEvent")
	pSetLayeredWindowAttributes    = user32.NewProc("SetLayeredWindowAttributes")
	pGetWindowLongPtrW             = user32.NewProc("GetWindowLongPtrW")
	pSetWindowLongPtrW             = user32.NewProc("SetWindowLongPtrW")
	pGetDpiForWindow               = user32.NewProc("GetDpiForWindow")
	pSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	pMonitorFromRect               = user32.NewProc("MonitorFromRect")
	pGetMonitorInfoW               = user32.NewProc("GetMonitorInfoW")
	pRegisterWindowMessageW        = user32.NewProc("RegisterWindowMessageW")
	pFindWindowW                   = user32.NewProc("FindWindowW")
	pFillRect                      = user32.NewProc("FillRect")
	pDrawTextW                     = user32.NewProc("DrawTextW")
	pGetSystemMetricsForDpi        = user32.NewProc("GetSystemMetricsForDpi")

	pCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	pCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	pSelectObject           = gdi32.NewProc("SelectObject")
	pDeleteObject           = gdi32.NewProc("DeleteObject")
	pDeleteDC               = gdi32.NewProc("DeleteDC")
	pBitBlt                 = gdi32.NewProc("BitBlt")
	pCreateFontW            = gdi32.NewProc("CreateFontW")
	pSetTextColor           = gdi32.NewProc("SetTextColor")
	pSetBkMode              = gdi32.NewProc("SetBkMode")
	pGetTextExtentPoint32W  = gdi32.NewProc("GetTextExtentPoint32W")
	pGetTextMetricsW        = gdi32.NewProc("GetTextMetricsW")
	pCreateSolidBrush       = gdi32.NewProc("CreateSolidBrush")
	pRoundRect              = gdi32.NewProc("RoundRect")
	pGetStockObject         = gdi32.NewProc("GetStockObject")
	pSetTextCharacterExtra  = gdi32.NewProc("SetTextCharacterExtra")
	pSetTextAlign           = gdi32.NewProc("SetTextAlign")
	pTextOutW               = gdi32.NewProc("TextOutW")

	pShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")

	pDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")

	pRegOpenKeyExW    = advapi32.NewProc("RegOpenKeyExW")
	pRegQueryValueExW = advapi32.NewProc("RegQueryValueExW")
	pRegSetValueExW   = advapi32.NewProc("RegSetValueExW")
	pRegDeleteValueW  = advapi32.NewProc("RegDeleteValueW")
	pRegCloseKey      = advapi32.NewProc("RegCloseKey")
)

func systemDir() string {
	buf := make([]uint16, 260)
	n, _, _ := pGetSystemDirectoryW.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 || int(n) > len(buf) {
		return `C:\Windows\System32`
	}
	return syscall.UTF16ToString(buf[:n])
}

func sysDLL(name string) *syscall.LazyDLL { return syscall.NewLazyDLL(sysDir + `\` + name) }

// ── Constants ───────────────────────────────────────────────────────────
const (
	WM_CREATE            = 0x0001
	WM_DESTROY           = 0x0002
	WM_ACTIVATE          = 0x0006
	WM_ENDSESSION        = 0x0016
	WM_PAINT             = 0x000F
	WM_CLOSE             = 0x0010
	WM_ERASEBKGND        = 0x0014
	WM_SETTINGCHANGE     = 0x001A
	WM_TIMECHANGE        = 0x001E
	WM_MOUSEACTIVATE     = 0x0021
	WM_WINDOWPOSCHANGING = 0x0046
	WM_CONTEXTMENU       = 0x007B
	WM_DISPLAYCHANGE     = 0x007E
	WM_NCLBUTTONDOWN     = 0x00A1
	WM_NULL              = 0x0000
	WM_TIMER             = 0x0113
	WM_QUIT              = 0x0012
	WM_MOUSEMOVE         = 0x0200
	WM_LBUTTONDOWN       = 0x0201
	WM_LBUTTONUP         = 0x0202
	WM_RBUTTONUP         = 0x0205
	WM_POWERBROADCAST    = 0x0218
	WM_EXITSIZEMOVE      = 0x0232
	WM_MOUSELEAVE        = 0x02A3
	WM_DPICHANGED        = 0x02E0
	WM_APP               = 0x8000

	WS_POPUP         = 0x80000000
	WS_EX_TOPMOST    = 0x00000008
	WS_EX_TOOLWINDOW = 0x00000080
	WS_EX_LAYERED    = 0x00080000

	SW_HIDE           = 0
	SW_SHOWNOACTIVATE = 4

	SWP_NOSIZE       = 0x0001
	SWP_NOMOVE       = 0x0002
	SWP_NOZORDER     = 0x0004
	SWP_NOACTIVATE   = 0x0010
	SWP_FRAMECHANGED = 0x0020

	HTCAPTION     = 2
	MA_NOACTIVATE = 3

	MF_STRING    = 0x0000
	MF_GRAYED    = 0x0001
	MF_CHECKED   = 0x0008
	MF_POPUP     = 0x0010
	MF_SEPARATOR = 0x0800
	MF_BYCOMMAND = 0x0000

	TPM_RIGHTBUTTON = 0x0002
	TPM_RETURNCMD   = 0x0100

	TME_LEAVE = 0x00000002
	LWA_ALPHA = 0x00000002

	DT_LEFT         = 0x0000
	DT_CENTER       = 0x0001
	DT_RIGHT        = 0x0002
	DT_VCENTER      = 0x0004
	DT_SINGLELINE   = 0x0020
	DT_NOPREFIX     = 0x0800
	DT_END_ELLIPSIS = 0x8000

	TA_LEFT     = 0
	TA_BASELINE = 24

	TRANSPARENT = 1
	NULL_PEN    = 8
	SRCCOPY     = 0x00CC0020

	NIM_ADD     = 0
	NIM_MODIFY  = 1
	NIM_DELETE  = 2
	NIF_MESSAGE = 0x1
	NIF_ICON    = 0x2
	NIF_TIP     = 0x4

	IMAGE_ICON  = 1
	SM_CXSMICON = 49
	SM_CXICON   = 11

	HKEY_CURRENT_USER = 0x80000001
	KEY_READ          = 0x20019
	KEY_WRITE         = 0x20006
	REG_SZ            = 1
	REG_DWORD         = 4
	REG_BINARY        = 3

	REG_NOTIFY_CHANGE_LAST_SET = 0x4
	PM_REMOVE                  = 0x1
	QS_ALLINPUT                = 0x04FF
	MWMO_INPUTAVAILABLE        = 0x4
	INFINITE                   = 0xFFFFFFFF

	MONITOR_DEFAULTTONEAREST = 2
	MONITOR_DEFAULTTOPRIMARY = 1

	ERROR_ALREADY_EXISTS = 183

	PBT_APMRESUMESUSPEND   = 0x7
	PBT_APMRESUMEAUTOMATIC = 0x12
)

var (
	HWND_TOP           uintptr = 0
	HWND_BOTTOM        uintptr = 1
	HWND_TOPMOST               = negPtr(1)
	HWND_NOTOPMOST             = negPtr(2)
	GWL_EXSTYLE                = negPtr(20)
	DPI_PER_MONITOR_V2         = negPtr(4)
)

func negPtr(n int) uintptr { v := -int64(n); return uintptr(v) }

// ── Structs ─────────────────────────────────────────────────────────────
type POINT struct{ X, Y int32 }
type RECT struct{ Left, Top, Right, Bottom int32 }
type SIZE struct{ CX, CY int32 }

type WNDCLASSEXW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type MSG struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      POINT
	private uint32
}

type PAINTSTRUCT struct {
	hdc         uintptr
	fErase      int32
	rcPaint     RECT
	fRestore    int32
	fIncUpdate  int32
	rgbReserved [32]byte
}

type TRACKMOUSEEVENT struct {
	cbSize      uint32
	dwFlags     uint32
	hwndTrack   uintptr
	dwHoverTime uint32
}

type MONITORINFO struct {
	cbSize    uint32
	rcMonitor RECT
	rcWork    RECT
	dwFlags   uint32
}

type WINDOWPOS struct {
	hwnd            uintptr
	hwndInsertAfter uintptr
	x, y, cx, cy    int32
	flags           uint32
}

type TEXTMETRICW struct {
	tmHeight, tmAscent, tmDescent, tmInternalLeading, tmExternalLeading int32
	tmAveCharWidth, tmMaxCharWidth, tmWeight, tmOverhang                int32
	tmDigitizedAspectX, tmDigitizedAspectY                              int32
	tmFirstChar, tmLastChar, tmDefaultChar, tmBreakChar                 uint16
	tmItalic, tmUnderlined, tmStruckOut, tmPitchAndFamily, tmCharSet    uint8
}

type GUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

type NOTIFYICONDATAW struct {
	cbSize           uint32
	hWnd             uintptr
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            uintptr
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uVersion         uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guidItem         GUID
	hBalloonIcon     uintptr
}

type SYSTEMTIME [8]uint16

type DYNAMIC_TIME_ZONE_INFORMATION struct {
	Bias                        int32
	StandardName                [32]uint16
	StandardDate                SYSTEMTIME
	StandardBias                int32
	DaylightName                [32]uint16
	DaylightDate                SYSTEMTIME
	DaylightBias                int32
	TimeZoneKeyName             [128]uint16
	DynamicDaylightTimeDisabled uint8
}

// ── Helpers ─────────────────────────────────────────────────────────────
func u16(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		p, _ = syscall.UTF16PtrFromString("?")
	}
	return p
}

func u16s(s string) []uint16 {
	b, err := syscall.UTF16FromString(s)
	if err != nil {
		return []uint16{0}
	}
	return b
}

func rgb(c uint32) uintptr { // 0xRRGGBB → COLORREF (0x00BBGGRR)
	r, g, b := c>>16&0xFF, c>>8&0xFF, c&0xFF
	return uintptr(r | g<<8 | b<<16)
}

func loword(v uintptr) int { return int(int16(v & 0xFFFF)) }
func hiword(v uintptr) int { return int(int16((v >> 16) & 0xFFFF)) }

func fillRect(hdc uintptr, l, t, r, b int, color uint32) {
	br, _, _ := pCreateSolidBrush.Call(rgb(color))
	rc := RECT{int32(l), int32(t), int32(r), int32(b)}
	pFillRect.Call(hdc, uintptr(unsafe.Pointer(&rc)), br)
	pDeleteObject.Call(br)
}

func roundRect(hdc uintptr, l, t, r, b, radius int, color uint32) {
	br, _, _ := pCreateSolidBrush.Call(rgb(color))
	oldB, _, _ := pSelectObject.Call(hdc, br)
	pen, _, _ := pGetStockObject.Call(NULL_PEN)
	oldP, _, _ := pSelectObject.Call(hdc, pen)
	pRoundRect.Call(hdc, uintptr(l), uintptr(t), uintptr(r+1), uintptr(b+1), uintptr(radius*2), uintptr(radius*2))
	pSelectObject.Call(hdc, oldP)
	pSelectObject.Call(hdc, oldB)
	pDeleteObject.Call(br)
}

func drawText(hdc uintptr, s string, l, t, r, b int, color uint32, flags uint32) {
	pSetTextColor.Call(hdc, rgb(color))
	rc := RECT{int32(l), int32(t), int32(r), int32(b)}
	w := u16s(s)
	pDrawTextW.Call(hdc, uintptr(unsafe.Pointer(&w[0])), uintptr(len(w)-1), uintptr(unsafe.Pointer(&rc)),
		uintptr(flags|DT_SINGLELINE|DT_NOPREFIX|DT_VCENTER))
}

func textOutBaseline(hdc uintptr, s string, x, baseline int, color uint32) {
	pSetTextColor.Call(hdc, rgb(color))
	pSetTextAlign.Call(hdc, TA_LEFT|TA_BASELINE)
	w := u16s(s)
	pTextOutW.Call(hdc, uintptr(x), uintptr(baseline), uintptr(unsafe.Pointer(&w[0])), uintptr(len(w)-1))
	pSetTextAlign.Call(hdc, TA_LEFT)
}

func textWidth(hdc uintptr, s string) int {
	w := u16s(s)
	var sz SIZE
	pGetTextExtentPoint32W.Call(hdc, uintptr(unsafe.Pointer(&w[0])), uintptr(len(w)-1), uintptr(unsafe.Pointer(&sz)))
	return int(sz.CX)
}

func textMetrics(hdc uintptr) TEXTMETRICW {
	var tm TEXTMETRICW
	pGetTextMetricsW.Call(hdc, uintptr(unsafe.Pointer(&tm)))
	return tm
}

func setTracking(hdc uintptr, px int) { pSetTextCharacterExtra.Call(hdc, uintptr(px)) }

func createFont(heightPx, weight int) uintptr {
	h, _, _ := pCreateFontW.Call(negPtr(heightPx), 0, 0, 0, uintptr(weight), 0, 0, 0,
		1 /*DEFAULT_CHARSET*/, 0, 0, 5 /*CLEARTYPE_QUALITY*/, 0, uintptr(unsafe.Pointer(u16("Segoe UI"))))
	return h
}

// ── Registry ────────────────────────────────────────────────────────────
func regOpen(path string, access uint32) (uintptr, bool) {
	var hk uintptr
	r, _, _ := pRegOpenKeyExW.Call(HKEY_CURRENT_USER, uintptr(unsafe.Pointer(u16(path))), 0, uintptr(access), uintptr(unsafe.Pointer(&hk)))
	return hk, r == 0
}

func regGetString(path, name string) (string, bool) {
	hk, ok := regOpen(path, KEY_READ)
	if !ok {
		return "", false
	}
	defer pRegCloseKey.Call(hk)
	buf := make([]uint16, 1024)
	size := uint32(len(buf) * 2)
	var typ uint32
	r, _, _ := pRegQueryValueExW.Call(hk, uintptr(unsafe.Pointer(u16(name))), 0, uintptr(unsafe.Pointer(&typ)),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r != 0 || typ != REG_SZ {
		return "", false
	}
	return syscall.UTF16ToString(buf), true
}

func regGetDWORD(path, name string) (uint32, bool) {
	hk, ok := regOpen(path, KEY_READ)
	if !ok {
		return 0, false
	}
	defer pRegCloseKey.Call(hk)
	var v, typ uint32
	size := uint32(4)
	r, _, _ := pRegQueryValueExW.Call(hk, uintptr(unsafe.Pointer(u16(name))), 0, uintptr(unsafe.Pointer(&typ)),
		uintptr(unsafe.Pointer(&v)), uintptr(unsafe.Pointer(&size)))
	return v, r == 0 && typ == REG_DWORD
}

func regSetString(path, name, value string) bool {
	hk, ok := regOpen(path, KEY_WRITE)
	if !ok {
		return false
	}
	defer pRegCloseKey.Call(hk)
	data := u16s(value)
	r, _, _ := pRegSetValueExW.Call(hk, uintptr(unsafe.Pointer(u16(name))), 0, REG_SZ,
		uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)*2))
	return r == 0
}

func regDelete(path, name string) {
	if hk, ok := regOpen(path, KEY_WRITE); ok {
		pRegDeleteValueW.Call(hk, uintptr(unsafe.Pointer(u16(name))))
		pRegCloseKey.Call(hk)
	}
}

// ── Dark context menus (uxtheme ordinals used by Notepad++, Explorer etc.) ──
func setMenuDarkMode(dark bool) {
	if uxtheme.Load() != nil {
		return
	}
	h := uxtheme.Handle()
	setMode, _, _ := pGetProcAddress.Call(h, 135) // SetPreferredAppMode
	flush, _, _ := pGetProcAddress.Call(h, 136)   // FlushMenuThemes
	if setMode == 0 {
		return
	}
	mode := uintptr(3) // ForceLight
	if dark {
		mode = 2 // ForceDark
	}
	syscall.SyscallN(setMode, mode)
	if flush != 0 {
		syscall.SyscallN(flush)
	}
}
