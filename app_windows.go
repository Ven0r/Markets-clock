//go:build windows

package main

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	className = "MarketClockWidget"
	appName   = "MarketClock"
	runKey    = `Software\Microsoft\Windows\CurrentVersion\Run`
	themeKey  = `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`

	WM_TRAY   = WM_APP + 1
	WM_SHOWME = WM_APP + 2
	timerID   = 1
)

// Menu command IDs
const (
	cmdMarket      = 100 // + market index
	cmdExch        = 200
	cmdNext        = 201
	cmdHours       = 202
	cmdLabels      = 203
	cmdFooter      = 204
	cmdTZSystem    = 300
	cmdTZ          = 301 // + zone index
	cmd12h         = 400
	cmdThemeDark   = 410
	cmdThemeLight  = 411
	cmdThemeSystem = 412
	cmdSize        = 420 // + size index
	cmdOpacity     = 430 // + opacity index
	cmdTop         = 440
	cmdNormal      = 441
	cmdBottom      = 442
	cmdLock        = 443
	cmdResetPos    = 444
	cmdStartup     = 450
	cmdAllDesks    = 445
	cmdBringHere   = 446
	cmdHide        = 460
	cmdExit        = 461
)

var (
	sizeNames   = []string{"Tiny", "Small", "Normal", "Large"}
	sizePoints  = []float64{8, 9, 10.5, 12}
	opacityOpts = []int{100, 90, 80, 70, 60}
)

type palette struct{ bg, alt, banner, border, text, muted, accent, open, brk, closed uint32 }

var (
	darkPal  = palette{0x131722, 0x1A1E2A, 0x1E2433, 0x2A2E39, 0xE0E3EB, 0x7C808C, 0x5B8DEF, 0x22C38E, 0xF5A623, 0xF0616D}
	lightPal = palette{0xFFFFFF, 0xF5F7FB, 0xEAEFF8, 0xD6DCE6, 0x131722, 0x6E717C, 0x2962FF, 0x089981, 0xD98A0B, 0xE0313F}
)

func blend(a, b uint32, t float64) uint32 {
	ch := func(sh uint) uint32 {
		x, y := float64(a>>sh&0xFF), float64(b>>sh&0xFF)
		return uint32(math.Round(x+(y-x)*t)) << sh
	}
	return ch(16) | ch(8) | ch(0)
}

func round(f float64) int { return int(math.Round(f)) }

func has(p *syscall.LazyProc) bool { return p.Find() == nil }

type fontSet struct{ base, semi, label, big, foot uintptr }

const (
	colName = iota
	colExch
	colLocal
	colStatus
	colNext
	colHours
)

type column struct {
	kind  int
	x, w  int
	align uint32
	label string
}

type layout struct {
	w, h                  int
	pad, gap, track, line int
	titleY, titleH        int
	bigY, bigH            int
	headerH               int
	labelsY, labelsH      int
	rowsY, rowH           int
	footY, footH          int
	pillW, pillH, pillR   int
	cols                  []column
	bigAscent, bigHeight  int
	baseHeight            int
}

type App struct {
	hwnd, hinst  uintptr
	s            *Settings
	dpi          int
	fonts        fontSet
	lay          layout
	pal          palette
	dark         bool
	enabled      []*Market
	viewLoc      *time.Location
	viewCity     string
	hover        int
	tracking     bool
	win11        bool
	icon, iconSm uintptr
	trayAdded    bool
	msgTaskbar   uintptr
	nowOffset    time.Duration
	autostart    bool // launched by "Start with Windows"
	userHidden   bool // hidden from the tray icon / menu
	offDesk      bool // hidden because another virtual desktop is showing
	watch        deskWatcher
}

var app *App

func main() {
	runtime.LockOSThread()
	if has(pSetProcessDpiAwarenessContext) { // the manifest normally does this already
		pSetProcessDpiAwarenessContext.Call(DPI_PER_MONITOR_V2)
	}

	// One copy at a time: a second launch just brings the first one back.
	_, _, e := pCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(u16(`Local\MarketClock.SingleInstance`))))
	if e == syscall.Errno(ERROR_ALREADY_EXISTS) {
		if h, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(u16(className))), 0); h != 0 {
			pPostMessageW.Call(h, WM_SHOWME, 0, 0)
		}
		return
	}

	app = &App{hover: -1, s: loadSettings()}
	for _, arg := range os.Args[1:] {
		if strings.EqualFold(arg, "--autostart") {
			app.autostart = true
		}
	}
	if v := os.Getenv("MARKETCLOCK_NOW"); v != "" { // testing aid: pretend it's another time
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			app.nowOffset = time.Until(t)
		}
	}
	app.run()
}

func (a *App) now() time.Time { return time.Now().Add(a.nowOffset) }

func (a *App) run() {
	a.hinst, _, _ = pGetModuleHandleW.Call(0)
	a.msgTaskbar, _, _ = pRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(u16("TaskbarCreated"))))
	a.icon = a.loadIcon(32)

	cursor, _, _ := pLoadCursorW.Call(0, 32512) // IDC_ARROW
	wc := WNDCLASSEXW{
		lpfnWndProc:   syscall.NewCallback(wndProc),
		hInstance:     a.hinst,
		hIcon:         a.icon,
		hCursor:       cursor,
		lpszClassName: u16(className),
		hIconSm:       a.loadIcon(16),
	}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	if startupEnabled() {
		setStartup(true) // keep the entry current
	}
	if !a.createWindow() {
		return
	}

	// Started by hand: it lives on the desktop you started it on.
	// Started with Windows: it returns to the desktop it lived on before.
	if !a.autostart {
		if cur, ok := currentDesktop(); ok {
			a.s.DesktopID = cur.String()
			a.s.save()
		}
	}
	a.watch.arm()
	a.syncDesktop()
	a.applyVisibility()
	a.tick()

	// Message loop that also wakes up when the active virtual desktop changes.
	var m MSG
	for {
		n, handles := uintptr(0), uintptr(0)
		if a.watch.armed {
			n, handles = 1, uintptr(unsafe.Pointer(&a.watch.event))
		}
		r, _, _ := pMsgWaitForMultipleObjectsEx.Call(n, handles, INFINITE, QS_ALLINPUT, MWMO_INPUTAVAILABLE)
		if n == 1 && r == 0 { // WAIT_OBJECT_0: the desktop changed
			a.watch.arm() // re-arm first so no switch is missed
			a.syncDesktop()
			continue
		}
		for {
			got, _, _ := pPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, PM_REMOVE)
			if got == 0 {
				break
			}
			if m.message == WM_QUIT {
				return
			}
			pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
		}
	}
}

func (a *App) createWindow() bool {
	x, y := a.s.X, a.s.Y
	if !a.s.HasPos {
		x, y = 100, 100 // placed properly once the DPI is known
	}
	// A tool window: no taskbar button and not in Alt+Tab. Windows shows tool
	// windows on every desktop, so syncDesktop hides it away from its home desktop.
	hwnd, _, _ := pCreateWindowExW.Call(WS_EX_TOOLWINDOW, uintptr(unsafe.Pointer(u16(className))),
		uintptr(unsafe.Pointer(u16("Market Clock"))), WS_POPUP, uintptr(x), uintptr(y), 200, 100, 0, 0, a.hinst, 0)
	if hwnd == 0 {
		return false
	}
	a.hwnd = hwnd

	// Windows 11: rounded corners and a thin themed border drawn by the system.
	pref := uint32(2) // DWMWCP_ROUND
	if has(pDwmSetWindowAttribute) {
		r, _, _ := pDwmSetWindowAttribute.Call(a.hwnd, 33, uintptr(unsafe.Pointer(&pref)), 4)
		a.win11 = r == 0
	}

	a.dpi = a.windowDPI()
	firstRun := !a.s.HasPos // check before layout records the temporary position
	a.applyAll(false)
	if firstRun {
		a.moveToDefault()
	} else {
		a.clampToScreen()
	}
	a.addTray()
	return true
}

// syncDesktop hides the widget unless its home desktop is the one showing.
func (a *App) syncDesktop() {
	off := false
	if cur, ok := currentDesktop(); ok && !a.s.AllDesks {
		home, ok := parseGUID(a.s.DesktopID)
		if !ok || !desktopExists(home) { // first run, or its desktop was closed
			home = cur
			a.s.DesktopID = cur.String()
			a.s.save()
		}
		off = cur != home
	}
	if off != a.offDesk {
		a.offDesk = off
		a.applyVisibility()
	}
}

func (a *App) applyVisibility() {
	want := !a.userHidden && !a.offDesk
	if want == a.visible() {
		return
	}
	if want {
		pShowWindow.Call(a.hwnd, SW_SHOWNOACTIVATE)
		a.applyLayer()
		a.invalidate()
		a.updateTrayTip()
	} else {
		pShowWindow.Call(a.hwnd, SW_HIDE)
	}
}

// moveHere makes the desktop you're looking at the widget's home and shows it.
func (a *App) moveHere() {
	if cur, ok := currentDesktop(); ok {
		a.s.DesktopID = cur.String()
		a.s.save()
	}
	a.userHidden, a.offDesk = false, false
	a.applyVisibility()
}

func (a *App) loadIcon(size int) uintptr {
	cx := size
	if has(pGetSystemMetricsForDpi) && a.dpi > 0 {
		metric := uintptr(SM_CXSMICON)
		if size > 16 {
			metric = SM_CXICON
		}
		if v, _, _ := pGetSystemMetricsForDpi.Call(metric, uintptr(a.dpi)); v > 0 {
			cx = int(v)
		}
	}
	h, _, _ := pLoadImageW.Call(a.hinst, 1, IMAGE_ICON, uintptr(cx), uintptr(cx), 0)
	if h == 0 {
		h, _, _ = pLoadIconW.Call(0, 32512) // IDI_APPLICATION
	}
	return h
}

func (a *App) windowDPI() int {
	if has(pGetDpiForWindow) {
		if d, _, _ := pGetDpiForWindow.Call(a.hwnd); d > 0 {
			return int(d)
		}
	}
	return 96
}

func (a *App) scale(px float64) int { return round(px * float64(a.dpi) / 96) }

// ── Settings → window ───────────────────────────────────────────────────

// applyAll re-reads every setting: zone, theme, markets, fonts, layout, size, opacity.
func (a *App) applyAll(save bool) {
	a.resolveZone()
	a.applyTheme()
	a.enabled = a.enabled[:0]
	for _, m := range Markets {
		if a.s.Markets[m.Key] {
			a.enabled = append(a.enabled, m)
		}
	}
	a.buildFonts()
	a.computeLayout()
	a.resizeToLayout()
	a.applyOpacity()
	a.invalidate()
	if save {
		a.s.save()
	}
}

func (a *App) resolveZone() {
	if a.s.TZ == "" {
		a.viewLoc, a.viewCity = systemZone()
		return
	}
	a.viewLoc, a.viewCity = loadZone(a.s.TZ), CityOf(a.s.TZ)
}

func (a *App) applyTheme() {
	switch a.s.Theme {
	case "Light":
		a.dark = false
	case "System":
		v, ok := regGetDWORD(themeKey, "AppsUseLightTheme")
		a.dark = !(ok && v == 1)
	default:
		a.dark = true
	}
	a.pal = lightPal
	if a.dark {
		a.pal = darkPal
	}
	if a.win11 {
		c := uint32(rgb(a.pal.border))
		pDwmSetWindowAttribute.Call(a.hwnd, 34, uintptr(unsafe.Pointer(&c)), 4) // DWMWA_BORDER_COLOR
	}
	setMenuDarkMode(a.dark)
}

func (a *App) applyOpacity() {
	ex, _, _ := pGetWindowLongPtrW.Call(a.hwnd, GWL_EXSTYLE)
	if a.s.Opacity >= 100 {
		if ex&WS_EX_LAYERED != 0 {
			pSetWindowLongPtrW.Call(a.hwnd, GWL_EXSTYLE, ex&^WS_EX_LAYERED)
		}
	} else {
		if ex&WS_EX_LAYERED == 0 {
			pSetWindowLongPtrW.Call(a.hwnd, GWL_EXSTYLE, ex|WS_EX_LAYERED)
		}
		pSetLayeredWindowAttributes.Call(a.hwnd, 0, uintptr(a.s.Opacity*255/100), LWA_ALPHA)
	}
	pSetWindowPos.Call(a.hwnd, 0, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_NOZORDER|SWP_NOACTIVATE|SWP_FRAMECHANGED)
}

func (a *App) applyLayer() {
	switch a.s.Layer {
	case "top":
		pSetWindowPos.Call(a.hwnd, HWND_TOPMOST, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE)
	case "bottom":
		pSetWindowPos.Call(a.hwnd, HWND_NOTOPMOST, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE)
		pSetWindowPos.Call(a.hwnd, HWND_BOTTOM, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE)
	default:
		pSetWindowPos.Call(a.hwnd, HWND_NOTOPMOST, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE)
		pSetWindowPos.Call(a.hwnd, HWND_TOP, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE)
	}
}

func (a *App) invalidate() { pInvalidateRect.Call(a.hwnd, 0, 0) }

// tick refreshes the display and re-arms the timer for the next minute boundary.
func (a *App) tick() {
	a.invalidate()
	a.updateTrayTip()
	if !a.watch.armed {
		a.watch.arm() // virtual desktops may have been set up since
	}
	a.syncDesktop()
	n := a.now()
	ms := 60000 - (n.Second()*1000 + n.Nanosecond()/1e6) + 25
	pSetTimer.Call(a.hwnd, timerID, uintptr(ms), 0)
}

// ── Fonts & layout ──────────────────────────────────────────────────────

func (a *App) buildFonts() {
	for _, f := range []uintptr{a.fonts.base, a.fonts.semi, a.fonts.label, a.fonts.big, a.fonts.foot} {
		if f != 0 {
			pDeleteObject.Call(f)
		}
	}
	pt := sizePoints[1]
	for i, n := range sizeNames {
		if n == a.s.Size {
			pt = sizePoints[i]
		}
	}
	px := func(points float64) int { return round(points * float64(a.dpi) / 72) }
	a.fonts = fontSet{
		base:  createFont(px(pt), 400),
		semi:  createFont(px(pt), 600),
		label: createFont(px(math.Max(7, pt*0.8)), 600),
		big:   createFont(px(pt*2.35), 600),
		foot:  createFont(px(math.Max(7, pt*0.86)), 400),
	}
}

func (a *App) computeLayout() {
	dc, _, _ := pCreateCompatibleDC.Call(0)
	defer pDeleteDC.Call(dc)
	f := a.fonts
	use := func(h uintptr) TEXTMETRICW { pSelectObject.Call(dc, h); return textMetrics(dc) }

	tmBase := use(f.base)
	cy := int(tmBase.tmHeight)
	tmLabel := use(f.label)
	ly := int(tmLabel.tmHeight)
	tmBig := use(f.big)
	tmFoot := use(f.foot)

	L := layout{baseHeight: cy, bigAscent: int(tmBig.tmAscent), bigHeight: int(tmBig.tmHeight)}
	L.pad = round(float64(cy) * 0.9)
	L.gap = round(float64(cy) * 1.15)
	L.track = max(1, round(float64(ly)*0.09))
	L.line = max(1, a.scale(1))

	measure := func(font uintptr, tracking int, ss ...string) int {
		pSelectObject.Call(dc, font)
		setTracking(dc, tracking)
		w := 0
		for _, s := range ss {
			w = max(w, textWidth(dc, s))
		}
		setTracking(dc, 0)
		return w
	}
	label := func(s string) int { return measure(f.label, L.track, s) }

	timeT := "12:59 PM"
	if !a.s.Use12h {
		timeT = "23:59"
	}
	var names, exchs []string
	for _, m := range a.enabled {
		names = append(names, m.Name)
		exchs = append(exchs, m.Exch)
	}
	var locals []string
	for _, d := range []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"} {
		locals = append(locals, d+"  "+timeT)
	}

	L.pillH = cy + round(float64(cy)*0.2)
	L.pillR = max(2, round(float64(cy)*0.3))
	L.pillW = measure(f.semi, 0, "● CLOSED", "● BREAK") + 2*round(float64(cy)*0.6)

	add := func(kind int, w int, align uint32, lbl string) {
		if a.s.ShowLabels {
			w = max(w, label(lbl))
		}
		L.cols = append(L.cols, column{kind: kind, w: w, align: align, label: lbl})
	}
	add(colName, measure(f.base, 0, names...), DT_LEFT, "MARKET")
	if a.s.ShowExch {
		add(colExch, measure(f.base, 0, exchs...), DT_LEFT, "EXCH")
	}
	add(colLocal, measure(f.base, 0, locals...), DT_RIGHT, "LOCAL TIME")
	add(colStatus, L.pillW, DT_CENTER, "STATUS")
	if a.s.ShowNext {
		add(colNext, measure(f.base, 0, "Resumes in 23h 59m", "Closes in 23h 59m", "Opens in 2d 23h", "Break in 23h 59m"), DT_LEFT, "NEXT")
	}
	if a.s.ShowHours {
		add(colHours, measure(f.base, 0, timeT+" – "+timeT), DT_RIGHT, "SESSION · "+strings.ToUpper(a.viewCity))
	}
	x := L.pad
	for i := range L.cols {
		L.cols[i].x = x
		x += L.cols[i].w + L.gap
	}
	tableW := x - L.gap + L.pad

	// Header: title row, then the big clock with city/date on the right.
	ampm := ""
	if a.s.Use12h {
		ampm = " PM"
	}
	headerW := L.pad + label("MARKET CLOCK") + L.gap + label("10 / 10 OPEN") + L.pad
	bigW := L.pad + measure(f.big, 0, "12:59") + measure(f.semi, 0, ampm) + L.gap +
		max(measure(f.semi, 0, a.viewCity), measure(f.base, 0, "Wed, Sep 30")) + L.pad
	footW := L.pad + measure(f.foot, 0, "Exchange holidays not included · right-click for options") + L.pad
	L.w = max(tableW, headerW, bigW, footW)

	y := round(float64(L.pad) * 0.8)
	L.titleY, L.titleH = y, ly
	y += ly + round(float64(cy)*0.2)
	L.bigY, L.bigH = y, max(L.bigHeight, 2*cy)
	y += L.bigH + round(float64(L.pad)*0.55)
	L.headerH = y
	if a.s.ShowLabels {
		L.labelsY, L.labelsH = y, ly+round(float64(cy)*0.9)
		y += L.labelsH
	} else {
		y += round(float64(cy) * 0.3)
	}
	L.rowsY, L.rowH = y, cy+2*round(float64(cy)*0.42)
	y += len(a.enabled) * L.rowH
	if a.s.ShowFooter {
		L.footY, L.footH = y, int(tmFoot.tmHeight)+2*round(float64(cy)*0.5)
		y += L.footH
	} else {
		y += round(float64(cy) * 0.35)
	}
	L.h = y
	a.lay = L
}

// ── Position helpers ────────────────────────────────────────────────────

func (a *App) windowRect() RECT {
	var r RECT
	pGetWindowRect.Call(a.hwnd, uintptr(unsafe.Pointer(&r)))
	return r
}

func workArea(r RECT, flag uintptr) RECT {
	mon, _, _ := pMonitorFromRect.Call(uintptr(unsafe.Pointer(&r)), flag)
	mi := MONITORINFO{}
	mi.cbSize = uint32(unsafe.Sizeof(mi))
	pGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi)))
	return mi.rcWork
}

// resizeToLayout fits the window to its content. On the right/bottom half of
// a screen the right/bottom edge stays put, so the widget grows inward.
func (a *App) resizeToLayout() {
	r := a.windowRect()
	w, h := a.lay.w, a.lay.h
	x, y := int(r.Left), int(r.Top)
	wa := workArea(r, MONITOR_DEFAULTTONEAREST)
	if (r.Left+r.Right)/2 > (wa.Left+wa.Right)/2 {
		x = int(r.Right) - w
	}
	if (r.Top+r.Bottom)/2 > (wa.Top+wa.Bottom)/2 {
		y = int(r.Bottom) - h
	}
	pSetWindowPos.Call(a.hwnd, 0, uintptr(x), uintptr(y), uintptr(w), uintptr(h), SWP_NOZORDER|SWP_NOACTIVATE)
	a.clampToScreen()
}

func (a *App) clampToScreen() {
	r := a.windowRect()
	wa := workArea(r, MONITOR_DEFAULTTONEAREST)
	w, h := r.Right-r.Left, r.Bottom-r.Top
	x := min(max(r.Left, wa.Left), wa.Right-w)
	y := min(max(r.Top, wa.Top), wa.Bottom-h)
	x, y = max(x, wa.Left), max(y, wa.Top)
	if x != r.Left || y != r.Top {
		pSetWindowPos.Call(a.hwnd, 0, uintptr(x), uintptr(y), 0, 0, SWP_NOSIZE|SWP_NOZORDER|SWP_NOACTIVATE)
	}
	a.rememberPos()
}

func (a *App) moveToDefault() { // top-right corner of the main screen
	wa := workArea(RECT{0, 0, 1, 1}, MONITOR_DEFAULTTOPRIMARY)
	m := a.scale(16)
	x, y := int(wa.Right)-a.lay.w-m, int(wa.Top)+m
	pSetWindowPos.Call(a.hwnd, 0, uintptr(x), uintptr(y), uintptr(a.lay.w), uintptr(a.lay.h), SWP_NOZORDER|SWP_NOACTIVATE)
	a.dpi = a.windowDPI()
	a.applyAll(false)
	a.rememberPos()
	a.s.save()
}

func (a *App) rememberPos() {
	r := a.windowRect()
	a.s.X, a.s.Y, a.s.HasPos = int(r.Left), int(r.Top), true
}

// ── Tray icon ───────────────────────────────────────────────────────────

func (a *App) trayData() *NOTIFYICONDATAW {
	nid := &NOTIFYICONDATAW{hWnd: a.hwnd, uID: 1}
	nid.cbSize = uint32(unsafe.Sizeof(*nid))
	return nid
}

func (a *App) addTray() {
	nid := a.trayData()
	nid.uFlags = NIF_MESSAGE | NIF_ICON | NIF_TIP
	nid.uCallbackMessage = WM_TRAY
	nid.hIcon = a.loadIcon(16)
	copy(nid.szTip[:127], u16s("Market Clock"))
	r, _, _ := pShellNotifyIconW.Call(NIM_ADD, uintptr(unsafe.Pointer(nid)))
	a.trayAdded = r != 0
}

func (a *App) updateTrayTip() {
	if !a.trayAdded {
		return
	}
	n := a.now()
	open := 0
	for _, m := range a.enabled {
		if m.StateAt(n).Status == Open {
			open++
		}
	}
	nid := a.trayData()
	nid.uFlags = NIF_TIP
	copy(nid.szTip[:127], u16s(fmt.Sprintf("Market Clock · %d of %d open", open, len(a.enabled))))
	pShellNotifyIconW.Call(NIM_MODIFY, uintptr(unsafe.Pointer(nid)))
}

func (a *App) removeTray() {
	if a.trayAdded {
		pShellNotifyIconW.Call(NIM_DELETE, uintptr(unsafe.Pointer(a.trayData())))
		a.trayAdded = false
	}
}

func (a *App) visible() bool {
	v, _, _ := pIsWindowVisible.Call(a.hwnd)
	return v != 0
}

func (a *App) setVisible(show bool) {
	a.userHidden = !show
	a.applyVisibility()
	if show {
		a.tick()
	}
}

// ── Start with Windows ──────────────────────────────────────────────────

func exePath() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	return p
}

func startupEnabled() bool {
	v, ok := regGetString(runKey, appName)
	if !ok {
		return false
	}
	path := strings.TrimSpace(v)
	if strings.HasPrefix(path, `"`) {
		if i := strings.Index(path[1:], `"`); i >= 0 {
			path = path[1 : i+1]
		}
	}
	return strings.EqualFold(path, exePath())
}

func setStartup(on bool) {
	if on {
		regSetString(runKey, appName, `"`+exePath()+`" --autostart`)
	} else {
		regDelete(runKey, appName)
	}
}

// ── Menu ────────────────────────────────────────────────────────────────

func menuItem(m uintptr, id int, text string, checked bool) {
	f := uintptr(MF_STRING)
	if checked {
		f |= MF_CHECKED
	}
	pAppendMenuW.Call(m, f, uintptr(id), uintptr(unsafe.Pointer(u16(text))))
}

func menuSub(m, sub uintptr, text string) {
	pAppendMenuW.Call(m, MF_POPUP, sub, uintptr(unsafe.Pointer(u16(text))))
}

func menuSep(m uintptr) { pAppendMenuW.Call(m, MF_SEPARATOR, 0, 0) }

func menuRadio(m uintptr, first, last, sel int) {
	pCheckMenuRadioItem.Call(m, uintptr(first), uintptr(last), uintptr(sel), MF_BYCOMMAND)
}

func newMenu() uintptr { h, _, _ := pCreatePopupMenu.Call(); return h }

func (a *App) showMenu(x, y int) {
	s := a.s
	root := newMenu()
	if a.offDesk {
		menuItem(root, cmdBringHere, "Move the widget to this desktop", false)
		menuSep(root)
	}

	mk := newMenu()
	for i, m := range Markets {
		menuItem(mk, cmdMarket+i, m.MenuLabel, s.Markets[m.Key])
	}
	menuSub(root, mk, "Markets")

	cols := newMenu()
	menuItem(cols, cmdExch, "Exchange code", s.ShowExch)
	menuItem(cols, cmdNext, "Countdown", s.ShowNext)
	menuItem(cols, cmdHours, "Session hours in your time", s.ShowHours)
	menuItem(cols, cmdLabels, "Column labels", s.ShowLabels)
	menuItem(cols, cmdFooter, "Footer (hover for exchange hours)", s.ShowFooter)
	menuSub(root, cols, "Columns")

	tz := newMenu()
	_, sysCity := systemZone()
	menuItem(tz, cmdTZSystem, "Match Windows ("+sysCity+")", false)
	menuSep(tz)
	sel := cmdTZSystem
	for i, z := range ViewZones {
		menuItem(tz, cmdTZ+i, CityOf(z)+"   ("+z+")", false)
		if z == s.TZ {
			sel = cmdTZ + i
		}
	}
	if sel == cmdTZSystem {
		menuRadio(tz, cmdTZSystem, cmdTZSystem, cmdTZSystem)
	} else {
		menuRadio(tz, cmdTZ, cmdTZ+len(ViewZones)-1, sel)
	}
	menuSub(root, tz, "Your time zone")
	menuItem(root, cmd12h, "12-hour clock (AM/PM)", s.Use12h)
	menuSep(root)

	th := newMenu()
	menuItem(th, cmdThemeDark, "Dark", false)
	menuItem(th, cmdThemeLight, "Light", false)
	menuItem(th, cmdThemeSystem, "Match Windows", false)
	menuRadio(th, cmdThemeDark, cmdThemeSystem, map[string]int{"Dark": cmdThemeDark, "Light": cmdThemeLight, "System": cmdThemeSystem}[s.Theme])
	menuSub(root, th, "Theme")

	sz := newMenu()
	for i, n := range sizeNames {
		menuItem(sz, cmdSize+i, n, false)
		if n == s.Size {
			menuRadio(sz, cmdSize, cmdSize+len(sizeNames)-1, cmdSize+i)
		}
	}
	menuSub(root, sz, "Text size")

	op := newMenu()
	for i, o := range opacityOpts {
		menuItem(op, cmdOpacity+i, fmt.Sprintf("%d%%", o), false)
		if o == s.Opacity {
			menuRadio(op, cmdOpacity, cmdOpacity+len(opacityOpts)-1, cmdOpacity+i)
		}
	}
	menuSub(root, op, "Opacity")

	win := newMenu()
	menuItem(win, cmdTop, "Always on top", false)
	menuItem(win, cmdNormal, "Normal", false)
	menuItem(win, cmdBottom, "Behind other windows", false)
	menuRadio(win, cmdTop, cmdBottom, map[string]int{"top": cmdTop, "normal": cmdNormal, "bottom": cmdBottom}[s.Layer])
	menuSep(win)
	menuItem(win, cmdLock, "Lock position", s.Locked)
	menuItem(win, cmdResetPos, "Move to top-right corner", false)
	menuSep(win)
	menuItem(win, cmdAllDesks, "Show on all desktops", s.AllDesks)
	if !s.AllDesks {
		info := "Lives on the desktop you start it on"
		if home, ok := parseGUID(s.DesktopID); ok {
			if _, tracked := currentDesktop(); tracked {
				info = "Lives on: " + desktopName(home)
			}
		}
		pAppendMenuW.Call(win, MF_STRING|MF_GRAYED, 0, uintptr(unsafe.Pointer(u16(info))))
	}
	menuSub(root, win, "Window")

	menuItem(root, cmdStartup, "Start with Windows", startupEnabled())
	menuSep(root)
	if a.userHidden {
		menuItem(root, cmdHide, "Show widget", false)
	} else {
		menuItem(root, cmdHide, "Hide (click tray icon to show)", false)
	}
	menuItem(root, cmdExit, "Exit Market Clock", false)

	pSetForegroundWindow.Call(a.hwnd)
	cmd, _, _ := pTrackPopupMenu.Call(root, TPM_RETURNCMD|TPM_RIGHTBUTTON, uintptr(x), uintptr(y), 0, a.hwnd, 0)
	pPostMessageW.Call(a.hwnd, WM_NULL, 0, 0)
	pDestroyMenu.Call(root)
	if a.s.Layer == "bottom" {
		a.applyLayer()
	}
	a.command(int(cmd))
}

func (a *App) command(cmd int) {
	s := a.s
	switch {
	case cmd == 0:
		return
	case cmd >= cmdMarket && cmd < cmdMarket+len(Markets):
		k := Markets[cmd-cmdMarket].Key
		s.Markets[k] = !s.Markets[k]
	case cmd == cmdExch:
		s.ShowExch = !s.ShowExch
	case cmd == cmdNext:
		s.ShowNext = !s.ShowNext
	case cmd == cmdHours:
		s.ShowHours = !s.ShowHours
	case cmd == cmdLabels:
		s.ShowLabels = !s.ShowLabels
	case cmd == cmdFooter:
		s.ShowFooter = !s.ShowFooter
	case cmd == cmdTZSystem:
		s.TZ = ""
	case cmd >= cmdTZ && cmd < cmdTZ+len(ViewZones):
		s.TZ = ViewZones[cmd-cmdTZ]
	case cmd == cmd12h:
		s.Use12h = !s.Use12h
	case cmd == cmdThemeDark:
		s.Theme = "Dark"
	case cmd == cmdThemeLight:
		s.Theme = "Light"
	case cmd == cmdThemeSystem:
		s.Theme = "System"
	case cmd >= cmdSize && cmd < cmdSize+len(sizeNames):
		s.Size = sizeNames[cmd-cmdSize]
	case cmd >= cmdOpacity && cmd < cmdOpacity+len(opacityOpts):
		s.Opacity = opacityOpts[cmd-cmdOpacity]
	case cmd == cmdTop || cmd == cmdNormal || cmd == cmdBottom:
		s.Layer = map[int]string{cmdTop: "top", cmdNormal: "normal", cmdBottom: "bottom"}[cmd]
		a.applyLayer()
		s.save()
		return
	case cmd == cmdLock:
		s.Locked = !s.Locked
		s.save()
		return
	case cmd == cmdResetPos:
		a.moveToDefault()
		return
	case cmd == cmdAllDesks:
		s.AllDesks = !s.AllDesks
		s.save()
		a.moveHere() // switching back to one desktop: this one
		a.syncDesktop()
		return
	case cmd == cmdBringHere:
		a.moveHere()
		return
	case cmd == cmdStartup:
		setStartup(!startupEnabled())
		return
	case cmd == cmdHide:
		a.setVisible(a.userHidden)
		return
	case cmd == cmdExit:
		pDestroyWindow.Call(a.hwnd)
		return
	}
	a.applyAll(true)
}

// ── Window procedure ────────────────────────────────────────────────────

func wndProc(hwnd, msg, wp, lp uintptr) uintptr {
	a := app
	if a == nil || a.hwnd == 0 || hwnd != a.hwnd {
		r, _, _ := pDefWindowProcW.Call(hwnd, msg, wp, lp)
		return r
	}
	switch msg {
	case WM_PAINT:
		var ps PAINTSTRUCT
		hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		var rc RECT
		pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
		w, h := int(rc.Right), int(rc.Bottom)
		mdc, _, _ := pCreateCompatibleDC.Call(hdc)
		bmp, _, _ := pCreateCompatibleBitmap.Call(hdc, uintptr(w), uintptr(h))
		old, _, _ := pSelectObject.Call(mdc, bmp)
		a.paint(mdc, w, h)
		pBitBlt.Call(hdc, 0, 0, uintptr(w), uintptr(h), mdc, 0, 0, SRCCOPY)
		pSelectObject.Call(mdc, old)
		pDeleteObject.Call(bmp)
		pDeleteDC.Call(mdc)
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0

	case WM_ERASEBKGND:
		return 1

	case WM_TIMER:
		a.tick()
		return 0

	case WM_ENDSESSION: // signing out / shutting down
		if wp != 0 {
			a.s.save()
		}
		return 0

	case WM_TIMECHANGE:
		a.applyAll(false)
		a.tick()
		return 0

	case WM_POWERBROADCAST:
		if wp == PBT_APMRESUMEAUTOMATIC || wp == PBT_APMRESUMESUSPEND {
			a.tick()
		}
		return 1

	case WM_SETTINGCHANGE:
		if a.s.Theme == "System" {
			a.applyTheme()
			a.invalidate()
		}
		return 0

	case WM_DISPLAYCHANGE:
		a.clampToScreen()
		return 0

	case WM_DPICHANGED:
		a.dpi = loword(wp)
		a.buildFonts()
		a.computeLayout()
		rc := (*RECT)(unsafe.Pointer(lp))
		pSetWindowPos.Call(hwnd, 0, uintptr(rc.Left), uintptr(rc.Top), uintptr(a.lay.w), uintptr(a.lay.h), SWP_NOZORDER|SWP_NOACTIVATE)
		a.invalidate()
		return 0

	case WM_MOUSEMOVE:
		row := a.rowAt(hiword(lp))
		if row != a.hover {
			a.hover = row
			a.invalidate()
		}
		if !a.tracking {
			tme := TRACKMOUSEEVENT{dwFlags: TME_LEAVE, hwndTrack: hwnd}
			tme.cbSize = uint32(unsafe.Sizeof(tme))
			pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
			a.tracking = true
		}
		return 0

	case WM_MOUSELEAVE:
		a.tracking = false
		if a.hover != -1 {
			a.hover = -1
			a.invalidate()
		}
		return 0

	case WM_LBUTTONDOWN:
		if !a.s.Locked {
			pReleaseCapture.Call()
			pSendMessageW.Call(hwnd, WM_NCLBUTTONDOWN, HTCAPTION, 0)
		}
		return 0

	case WM_EXITSIZEMOVE:
		a.rememberPos()
		a.s.save()
		return 0

	case WM_MOUSEACTIVATE:
		if a.s.Layer == "bottom" {
			return MA_NOACTIVATE
		}

	case WM_WINDOWPOSCHANGING:
		if a.s.Layer == "bottom" {
			pos := (*WINDOWPOS)(unsafe.Pointer(lp))
			pos.hwndInsertAfter = HWND_BOTTOM
			pos.flags &^= SWP_NOZORDER
		}

	case WM_CONTEXTMENU:
		x, y := loword(lp), hiword(lp)
		if x == -1 && y == -1 {
			var pt POINT
			pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
			x, y = int(pt.X), int(pt.Y)
		}
		a.showMenu(x, y)
		return 0

	case WM_TRAY:
		switch loword(lp) {
		case WM_LBUTTONUP:
			switch {
			case a.offDesk:
				a.moveHere() // clicked from another desktop: bring it here
			default:
				a.setVisible(a.userHidden)
			}
		case WM_RBUTTONUP:
			var pt POINT
			pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
			a.showMenu(int(pt.X), int(pt.Y))
		}
		return 0

	case WM_SHOWME: // launched again while already running
		a.moveHere()
		return 0

	case WM_CLOSE:
		pDestroyWindow.Call(hwnd)
		return 0

	case WM_DESTROY:
		a.removeTray()
		a.s.save()
		pPostQuitMessage.Call(0)
		return 0
	}
	if msg == a.msgTaskbar && a.msgTaskbar != 0 { // Explorer restarted: put the tray icon back
		a.addTray()
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, msg, wp, lp)
	return r
}

func (a *App) rowAt(y int) int {
	L := a.lay
	if y < L.rowsY || L.rowH == 0 {
		return -1
	}
	i := (y - L.rowsY) / L.rowH
	if i >= len(a.enabled) {
		return -1
	}
	return i
}
