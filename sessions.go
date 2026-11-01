package main

// Session logic, ported from the Market Clock Pine Script.
// This file is plain Go (no Windows calls) so it can be unit-tested anywhere.

import (
	"fmt"
	"math"
	"strings"
	"time"
	_ "time/tzdata" // embed the IANA time zone database so DST rules never depend on the PC
)

// Market is one exchange. Session times are minutes after midnight in the
// exchange's OWN time zone (9:30 AM = 570). BreakStart = -1 means no lunch.
// Overnight = true: the session opens the evening before and runs into the
// next day (CME Globex: Sun–Thu 5:00 PM → next day 4:00 PM, Chicago time).
type Market struct {
	Key        string
	Name       string
	Exch       string
	MenuLabel  string
	TZ         string
	Open       int
	Close      int
	BreakStart int
	BreakEnd   int
	Overnight  bool
	DefaultOn  bool
	loc        *time.Location
}

var Markets = []*Market{
	{Key: "SYD", Name: "Sydney", Exch: "ASX", MenuLabel: "Sydney · ASX", TZ: "Australia/Sydney", Open: 600, Close: 960, BreakStart: -1, BreakEnd: -1, DefaultOn: true},
	{Key: "TKY", Name: "Tokyo", Exch: "TSE", MenuLabel: "Tokyo · TSE", TZ: "Asia/Tokyo", Open: 540, Close: 930, BreakStart: 690, BreakEnd: 750, DefaultOn: true},
	{Key: "HKG", Name: "Hong Kong", Exch: "HKEX", MenuLabel: "Hong Kong · HKEX", TZ: "Asia/Hong_Kong", Open: 570, Close: 960, BreakStart: 720, BreakEnd: 780, DefaultOn: true},
	{Key: "SHA", Name: "Shanghai", Exch: "SSE", MenuLabel: "Shanghai · SSE", TZ: "Asia/Shanghai", Open: 570, Close: 900, BreakStart: 690, BreakEnd: 780, DefaultOn: true},
	{Key: "BOM", Name: "Mumbai", Exch: "NSE", MenuLabel: "Mumbai · NSE", TZ: "Asia/Kolkata", Open: 555, Close: 930, BreakStart: -1, BreakEnd: -1, DefaultOn: true},
	{Key: "FRA", Name: "Frankfurt", Exch: "Xetra", MenuLabel: "Frankfurt · Xetra", TZ: "Europe/Berlin", Open: 540, Close: 1050, BreakStart: -1, BreakEnd: -1, DefaultOn: true},
	{Key: "PAR", Name: "Paris", Exch: "Euronext", MenuLabel: "Paris · Euronext", TZ: "Europe/Paris", Open: 540, Close: 1050, BreakStart: -1, BreakEnd: -1, DefaultOn: false},
	{Key: "LON", Name: "London", Exch: "LSE", MenuLabel: "London · LSE", TZ: "Europe/London", Open: 480, Close: 990, BreakStart: -1, BreakEnd: -1, DefaultOn: true},
	{Key: "NYC", Name: "New York", Exch: "NYSE", MenuLabel: "New York · NYSE", TZ: "America/New_York", Open: 570, Close: 960, BreakStart: -1, BreakEnd: -1, DefaultOn: true},
	{Key: "CME", Name: "Chicago", Exch: "CME", MenuLabel: "Chicago · CME Globex futures", TZ: "America/Chicago", Open: 1020, Close: 960, BreakStart: -1, BreakEnd: -1, Overnight: true, DefaultOn: true},
}

// Time zones offered in the menu (same list as the Pine input).
var ViewZones = []string{
	"America/Chicago", "America/New_York", "America/Denver", "America/Phoenix",
	"America/Los_Angeles", "America/Anchorage", "Pacific/Honolulu", "America/Toronto",
	"America/Sao_Paulo", "Europe/London", "Europe/Berlin", "Europe/Istanbul",
	"Asia/Dubai", "Asia/Kolkata", "Asia/Singapore", "Asia/Hong_Kong", "Asia/Tokyo",
	"Australia/Sydney", "UTC",
}

func init() {
	for _, m := range Markets {
		m.loc = loadZone(m.TZ)
	}
}

func loadZone(name string) *time.Location {
	if loc, err := time.LoadLocation(name); err == nil {
		return loc
	}
	return time.UTC
}

type Status int

const (
	Closed Status = iota
	Open
	Break
)

func (s Status) String() string {
	switch s {
	case Open:
		return "OPEN"
	case Break:
		return "BREAK"
	}
	return "CLOSED"
}

// State is what one row of the table shows.
type State struct {
	Status    Status
	Verb      string    // "Opens in", "Closes in", "Break in", "Resumes in"
	Next      time.Time // time of the next status change
	SessOpen  time.Time // the current or next session's open
	SessClose time.Time // ...and its close
}

// at returns minOfDay on the calendar day `offset` days after the day that
// contains `ref`, in loc. time.Date normalises the date, so this is DST-safe.
func at(loc *time.Location, ref time.Time, offset, minOfDay int) time.Time {
	y, mo, d := ref.In(loc).Date()
	return time.Date(y, mo, d+offset, minOfDay/60, minOfDay%60, 0, 0, loc)
}

func isWeekday(d time.Weekday) bool { return d >= time.Monday && d <= time.Friday }

// nextOpen is the next time this market opens after now (skips weekends).
func (m *Market) nextOpen(now time.Time) time.Time {
	for i := 0; i <= 8; i++ {
		t := at(m.loc, now, i, m.Open)
		d := t.Weekday()
		ok := isWeekday(d)
		if m.Overnight {
			ok = d <= time.Thursday // Sun–Thu evenings
		}
		if ok && t.After(now) {
			return t
		}
	}
	return time.Time{}
}

// StateAt works out the market's status at `now`.
func (m *Market) StateAt(now time.Time) State {
	loc := m.loc
	lt := now.In(loc)
	dow := lt.Weekday()
	mins := lt.Hour()*60 + lt.Minute()
	weekday := isWeekday(dow)
	s := State{Status: Closed, Verb: "Opens in"}

	if !m.Overnight {
		if weekday && mins >= m.Open && mins < m.Close {
			hasBreak := m.BreakStart >= 0
			s.Status = Open
			s.SessOpen = at(loc, now, 0, m.Open)
			s.SessClose = at(loc, now, 0, m.Close)
			switch {
			case hasBreak && mins >= m.BreakStart && mins < m.BreakEnd:
				s.Status, s.Verb, s.Next = Break, "Resumes in", at(loc, now, 0, m.BreakEnd)
			case hasBreak && mins < m.BreakStart:
				s.Verb, s.Next = "Break in", at(loc, now, 0, m.BreakStart)
			default:
				s.Verb, s.Next = "Closes in", s.SessClose
			}
		}
	} else {
		eve := mins >= m.Open && dow <= time.Thursday // Sun–Thu evening
		day := mins < m.Close && weekday              // Mon–Fri day
		switch {
		case eve || day:
			s.Status = Open
			if eve {
				s.SessOpen, s.SessClose = at(loc, now, 0, m.Open), at(loc, now, 1, m.Close)
			} else {
				s.SessOpen, s.SessClose = at(loc, now, -1, m.Open), at(loc, now, 0, m.Close)
			}
			s.Verb = "Break in"
			if s.SessClose.Weekday() == time.Friday {
				s.Verb = "Closes in"
			}
			s.Next = s.SessClose
		case weekday && dow != time.Friday:
			// Mon–Thu daily maintenance break
			s.Status, s.Verb = Break, "Resumes in"
			s.Next = at(loc, now, 0, m.Open)
			s.SessOpen, s.SessClose = s.Next, at(loc, now, 1, m.Close)
		}
	}

	if s.Status == Closed {
		s.Next = m.nextOpen(now)
		s.SessOpen = s.Next
		if !s.Next.IsZero() {
			off := 0
			if m.Overnight {
				off = 1
			}
			s.SessClose = at(loc, s.Next, off, m.Close)
		}
	}
	return s
}

// ── Formatting ──────────────────────────────────────────────────────────

// FmtClock gives "7:05 PM" or "19:05".
func FmtClock(t time.Time, loc *time.Location, use12h bool) string {
	if t.IsZero() {
		return "—"
	}
	lt := t.In(loc)
	return fmtHM(lt.Hour(), lt.Minute(), use12h)
}

func fmtHM(h, m int, use12h bool) string {
	if !use12h {
		return fmt.Sprintf("%02d:%02d", h, m)
	}
	hh := h % 12
	if hh == 0 {
		hh = 12
	}
	ap := "AM"
	if h >= 12 {
		ap = "PM"
	}
	return fmt.Sprintf("%d:%02d %s", hh, m, ap)
}

func fmtMin(minOfDay int, use12h bool) string { return fmtHM(minOfDay/60, minOfDay%60, use12h) }

// FmtDur gives "2h 05m", "45m" or "1d 6h" (minutes rounded up, like the Pine script).
func FmtDur(d time.Duration) string {
	total := int(math.Ceil(d.Minutes()))
	if total < 0 {
		total = 0
	}
	days, h, m := total/1440, (total%1440)/60, total%60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, h)
	case h > 0:
		return fmt.Sprintf("%dh %02dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

// HoursText is the hover detail: the exchange's hours in its own local time.
func (m *Market) HoursText(use12h bool) string {
	city := CityOf(m.TZ)
	base := fmt.Sprintf("%s · %s time %s – %s", m.Exch, city, fmtMin(m.Open, use12h), fmtMin(m.Close, use12h))
	switch {
	case m.Overnight:
		return fmt.Sprintf("%s · %s time %s – %s next day, Sun–Fri · daily break %s – %s",
			m.Exch+" Globex", city, fmtMin(m.Open, use12h), fmtMin(m.Close, use12h),
			fmtMin(m.Close, use12h), fmtMin(m.Open, use12h))
	case m.BreakStart >= 0:
		return fmt.Sprintf("%s · lunch %s – %s", base, fmtMin(m.BreakStart, use12h), fmtMin(m.BreakEnd, use12h))
	}
	return base
}

// CityOf turns "America/Los_Angeles" into "Los Angeles".
func CityOf(tz string) string {
	parts := strings.Split(tz, "/")
	return strings.ReplaceAll(parts[len(parts)-1], "_", " ")
}

func DayName(t time.Time, loc *time.Location) string { return t.In(loc).Format("Mon") }
