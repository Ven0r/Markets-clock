//go:build windows

package main

import (
	"fmt"
	"strings"
	"time"
)

func (a *App) paint(hdc uintptr, w, h int) {
	p, L, s := a.pal, a.lay, a.s
	f := a.fonts
	now := a.now()
	sel := func(font uintptr) { pSelectObject.Call(hdc, font) }

	pSetBkMode.Call(hdc, TRANSPARENT)
	fillRect(hdc, 0, 0, w, h, p.bg)
	fillRect(hdc, 0, 0, w, L.headerH, p.banner)
	fillRect(hdc, 0, L.headerH, w, L.headerH+L.line, p.border)

	states := make([]State, len(a.enabled))
	open := 0
	for i, m := range a.enabled {
		states[i] = m.StateAt(now)
		if states[i].Status == Open {
			open++
		}
	}

	// Title row: MARKET CLOCK ........ 7 / 9 OPEN
	sel(f.label)
	setTracking(hdc, L.track)
	drawText(hdc, "MARKET CLOCK", L.pad, L.titleY, w-L.pad, L.titleY+L.titleH, p.accent, DT_LEFT)
	countCol := p.muted
	if open > 0 {
		countCol = p.open
	}
	drawText(hdc, fmt.Sprintf("%d / %d OPEN", open, len(a.enabled)), L.pad, L.titleY, w-L.pad, L.titleY+L.titleH, countCol, DT_RIGHT)
	setTracking(hdc, 0)

	// Big clock, with AM/PM set smaller on the same baseline
	clock := FmtClock(now, a.viewLoc, s.Use12h)
	digits, ampm := clock, ""
	if i := strings.IndexByte(clock, ' '); i > 0 {
		digits, ampm = clock[:i], clock[i:]
	}
	baseline := L.bigY + (L.bigH-L.bigHeight)/2 + L.bigAscent
	sel(f.big)
	textOutBaseline(hdc, digits, L.pad, baseline, p.text)
	dw := textWidth(hdc, digits)
	if ampm != "" {
		sel(f.semi)
		textOutBaseline(hdc, ampm, L.pad+dw, baseline, p.muted)
	}
	// City and date, stacked on the right
	lt := now.In(a.viewLoc)
	top := L.bigY + (L.bigH-2*L.baseHeight)/2
	sel(f.semi)
	drawText(hdc, a.viewCity, L.pad, top, w-L.pad, top+L.baseHeight, p.text, DT_RIGHT)
	sel(f.base)
	drawText(hdc, lt.Format("Mon, Jan 2"), L.pad, top+L.baseHeight, w-L.pad, top+2*L.baseHeight, p.muted, DT_RIGHT)

	// Column labels
	if s.ShowLabels {
		sel(f.label)
		setTracking(hdc, L.track)
		for _, c := range L.cols {
			drawText(hdc, c.label, c.x, L.labelsY, c.x+c.w, L.labelsY+L.labelsH, p.muted, c.align)
		}
		setTracking(hdc, 0)
	}

	// One row per market
	for i, m := range a.enabled {
		st := states[i]
		y0, y1 := L.rowsY+i*L.rowH, L.rowsY+(i+1)*L.rowH
		rowBg := p.bg
		if i%2 == 0 {
			rowBg = p.alt
		}
		if i == a.hover {
			rowBg = blend(rowBg, p.accent, 0.12)
		}
		fillRect(hdc, 0, y0, w, y1, rowBg)

		stCol, tint := p.closed, 0.12
		switch st.Status {
		case Open:
			stCol, tint = p.open, 0.22
		case Break:
			stCol, tint = p.brk, 0.20
		}

		for _, c := range L.cols {
			sel(f.base)
			switch c.kind {
			case colName:
				col := p.muted
				if st.Status == Open {
					col = p.text
				}
				drawText(hdc, m.Name, c.x, y0, c.x+c.w, y1, col, c.align)
			case colExch:
				drawText(hdc, m.Exch, c.x, y0, c.x+c.w, y1, p.muted, c.align)
			case colLocal:
				drawText(hdc, DayName(now, m.loc)+"  "+FmtClock(now, m.loc, s.Use12h), c.x, y0, c.x+c.w, y1, p.text, c.align)
			case colStatus:
				px := c.x + (c.w-L.pillW)/2
				py := y0 + (L.rowH-L.pillH)/2
				roundRect(hdc, px, py, px+L.pillW, py+L.pillH, L.pillR, blend(rowBg, stCol, tint))
				sel(f.semi)
				drawText(hdc, "● "+st.Status.String(), px, py, px+L.pillW, py+L.pillH, stCol, DT_CENTER)
			case colNext:
				left := st.Next.Sub(now)
				col := p.muted
				if !st.Next.IsZero() && left <= time.Hour {
					col = p.brk
				}
				drawText(hdc, st.Verb+" "+FmtDur(left), c.x, y0, c.x+c.w, y1, col, c.align)
			case colHours:
				drawText(hdc, FmtClock(st.SessOpen, a.viewLoc, s.Use12h)+" – "+FmtClock(st.SessClose, a.viewLoc, s.Use12h),
					c.x, y0, c.x+c.w, y1, p.muted, c.align)
			}
		}
	}

	// Footer: the hovered exchange's own hours, otherwise a hint
	if s.ShowFooter {
		fillRect(hdc, 0, L.footY, w, L.footY+L.line, p.border)
		sel(f.foot)
		text, col := "Exchange holidays not included · right-click for options", p.muted
		if a.hover >= 0 && a.hover < len(a.enabled) {
			text, col = a.enabled[a.hover].HoursText(s.Use12h), p.text
		}
		drawText(hdc, text, L.pad, L.footY+L.line, w-L.pad, L.footY+L.footH, col, DT_LEFT|DT_END_ELLIPSIS)
	}

	// Windows 10 has no system-drawn border on this kind of window, so draw one.
	if !a.win11 {
		fillRect(hdc, 0, 0, w, L.line, p.border)
		fillRect(hdc, 0, h-L.line, w, h, p.border)
		fillRect(hdc, 0, 0, L.line, h, p.border)
		fillRect(hdc, w-L.line, 0, w, h, p.border)
	}
}
