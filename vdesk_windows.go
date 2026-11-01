//go:build windows

package main

// Keeping the widget on one virtual desktop.
//
// Windows only pins regular taskbar windows to a single desktop; a widget kept
// off the taskbar is shown on every desktop no matter what. So the widget
// follows the active desktop itself: Explorer records it in the registry
// (CurrentVirtualDesktop), Windows notifies us the moment that value changes,
// and the widget hides on every desktop except its home one.

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"unsafe"
)

const vdKey = `Software\Microsoft\Windows\CurrentVersion\Explorer\VirtualDesktops`

// Older Windows 10 builds keep the current desktop under a per-session key.
func sessionVDKey() string {
	var sid uint32
	pid, _, _ := pGetCurrentProcessId.Call()
	pProcessIdToSessionId.Call(pid, uintptr(unsafe.Pointer(&sid)))
	return fmt.Sprintf(`Software\Microsoft\Windows\CurrentVersion\Explorer\SessionInfo\%d\VirtualDesktops`, sid)
}

func regGetBinary(path, name string) ([]byte, bool) {
	hk, ok := regOpen(path, KEY_READ)
	if !ok {
		return nil, false
	}
	defer pRegCloseKey.Call(hk)
	buf := make([]byte, 4096)
	size := uint32(len(buf))
	var typ uint32
	r, _, _ := pRegQueryValueExW.Call(hk, uintptr(unsafe.Pointer(u16(name))), 0, uintptr(unsafe.Pointer(&typ)),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r != 0 || typ != REG_BINARY {
		return nil, false
	}
	return buf[:size], true
}

func guidFromBytes(b []byte) GUID {
	var g GUID
	g.Data1 = binary.LittleEndian.Uint32(b[0:4])
	g.Data2 = binary.LittleEndian.Uint16(b[4:6])
	g.Data3 = binary.LittleEndian.Uint16(b[6:8])
	copy(g.Data4[:], b[8:16])
	return g
}

func regGetGUID(path, name string) (GUID, bool) {
	b, ok := regGetBinary(path, name)
	if !ok || len(b) < 16 {
		return GUID{}, false
	}
	g := guidFromBytes(b)
	return g, g != GUID{}
}

// currentDesktop is the virtual desktop being shown, if Windows has recorded one
// (it may not have until a second desktop is created).
func currentDesktop() (GUID, bool) {
	if g, ok := regGetGUID(vdKey, "CurrentVirtualDesktop"); ok {
		return g, true
	}
	return regGetGUID(sessionVDKey(), "CurrentVirtualDesktop")
}

func desktopList() []GUID {
	b, _ := regGetBinary(vdKey, "VirtualDesktopIDs")
	var out []GUID
	for i := 0; i+16 <= len(b); i += 16 {
		out = append(out, guidFromBytes(b[i:i+16]))
	}
	return out
}

func desktopExists(g GUID) bool {
	list := desktopList()
	if len(list) == 0 {
		return true // can't tell, so assume it does
	}
	for _, d := range list {
		if d == g {
			return true
		}
	}
	return false
}

// desktopName is the name shown in Task View ("Desktop 2" or a custom name).
func desktopName(g GUID) string {
	if n, ok := regGetString(vdKey+`\Desktops\{`+g.String()+`}`, "Name"); ok && n != "" {
		return n
	}
	for i, d := range desktopList() {
		if d == g {
			return fmt.Sprintf("Desktop %d", i+1)
		}
	}
	return "this desktop"
}

// deskWatcher gets a wake-up event whenever the current desktop changes.
type deskWatcher struct {
	hkey, event uintptr
	armed       bool
}

func (w *deskWatcher) arm() {
	w.armed = false
	if w.event == 0 {
		w.event, _, _ = pCreateEventW.Call(0, 0, 0, 0)
		if w.event == 0 {
			return
		}
	}
	if w.hkey == 0 {
		path := vdKey
		if _, ok := regGetGUID(vdKey, "CurrentVirtualDesktop"); !ok {
			if _, ok := regGetGUID(sessionVDKey(), "CurrentVirtualDesktop"); ok {
				path = sessionVDKey()
			}
		}
		hk, ok := regOpen(path, KEY_READ)
		if !ok {
			return // no virtual desktops recorded yet; retried every minute
		}
		w.hkey = hk
	}
	// One-shot and asynchronous: the event is signalled on the next change.
	r, _, _ := pRegNotifyChangeKeyValue.Call(w.hkey, 0, REG_NOTIFY_CHANGE_LAST_SET, w.event, 1)
	if r != 0 {
		pRegCloseKey.Call(w.hkey)
		w.hkey = 0
		return
	}
	w.armed = true
}

// ── GUID text form, for settings.json ───────────────────────────────────

func (g GUID) String() string {
	return fmt.Sprintf("%08X-%04X-%04X-%02X%02X-%02X%02X%02X%02X%02X%02X",
		g.Data1, g.Data2, g.Data3, g.Data4[0], g.Data4[1], g.Data4[2], g.Data4[3], g.Data4[4], g.Data4[5], g.Data4[6], g.Data4[7])
}

func parseGUID(s string) (GUID, bool) {
	h := strings.NewReplacer("{", "", "}", "", "-", "").Replace(s)
	if len(h) != 32 {
		return GUID{}, false
	}
	part := func(i, n int) (uint64, bool) {
		v, err := strconv.ParseUint(h[i:i+n], 16, 64)
		return v, err == nil
	}
	var g GUID
	d1, ok1 := part(0, 8)
	d2, ok2 := part(8, 4)
	d3, ok3 := part(12, 4)
	if !ok1 || !ok2 || !ok3 {
		return GUID{}, false
	}
	g.Data1, g.Data2, g.Data3 = uint32(d1), uint16(d2), uint16(d3)
	for i := 0; i < 8; i++ {
		b, ok := part(16+i*2, 2)
		if !ok {
			return GUID{}, false
		}
		g.Data4[i] = byte(b)
	}
	return g, g != GUID{}
}
