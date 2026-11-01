package main

import (
	"testing"
	"time"
)

var chicago = loadZone("America/Chicago")

func mkt(key string) *Market {
	for _, m := range Markets {
		if m.Key == key {
			return m
		}
	}
	panic("no market " + key)
}

func ct(y int, mo time.Month, d, h, min int) time.Time {
	return time.Date(y, mo, d, h, min, 0, 0, chicago)
}

type want struct {
	status  Status
	verb    string
	next    string // countdown text
	session string // "open – close" in Chicago time
}

func check(t *testing.T, label string, now time.Time, key string, w want) {
	t.Helper()
	m := mkt(key)
	s := m.StateAt(now)
	gotNext := FmtDur(s.Next.Sub(now))
	gotSess := FmtClock(s.SessOpen, chicago, true) + " – " + FmtClock(s.SessClose, chicago, true)
	if s.Status != w.status || s.Verb != w.verb || gotNext != w.next || (w.session != "" && gotSess != w.session) {
		t.Errorf("%s %s: got %v / %q %s / %s, want %v / %q %s / %s",
			label, key, s.Status, s.Verb, gotNext, gotSess, w.status, w.verb, w.next, w.session)
	}
}

func TestSundayBeforeOpen(t *testing.T) {
	now := ct(2026, 11, 1, 11, 53) // Sun, US DST just ended
	check(t, "Sun", now, "CME", want{Closed, "Opens in", "5h 07m", "5:00 PM – 4:00 PM"})
	check(t, "Sun", now, "NYC", want{Closed, "Opens in", "20h 37m", "8:30 AM – 3:00 PM"})
	check(t, "Sun", now, "SYD", want{Closed, "Opens in", "5h 07m", "5:00 PM – 11:00 PM"})
	check(t, "Sun", now, "TKY", want{Closed, "Opens in", "6h 07m", "6:00 PM – 12:30 AM"})
}

func TestTuesdayMorning(t *testing.T) {
	now := ct(2026, 11, 3, 10, 0)
	check(t, "Tue", now, "NYC", want{Open, "Closes in", "5h 00m", "8:30 AM – 3:00 PM"})
	check(t, "Tue", now, "CME", want{Open, "Break in", "6h 00m", "5:00 PM – 4:00 PM"})
	check(t, "Tue", now, "LON", want{Open, "Closes in", "30m", "2:00 AM – 10:30 AM"})
	check(t, "Tue", now, "FRA", want{Open, "Closes in", "30m", "2:00 AM – 10:30 AM"})
	check(t, "Tue", now, "TKY", want{Closed, "Opens in", "8h 00m", "6:00 PM – 12:30 AM"})
	check(t, "Tue", now, "BOM", want{Closed, "Opens in", "11h 45m", "9:45 PM – 4:00 AM"})
}

func TestCMEMaintenanceBreak(t *testing.T) {
	check(t, "Tue 4:30p", ct(2026, 11, 3, 16, 30), "CME", want{Break, "Resumes in", "30m", "5:00 PM – 4:00 PM"})
}

func TestCMEFridayAndWeekend(t *testing.T) {
	check(t, "Fri 3p", ct(2026, 11, 6, 15, 0), "CME", want{Open, "Closes in", "1h 00m", ""})
	check(t, "Fri 4:30p", ct(2026, 11, 6, 16, 30), "CME", want{Closed, "Opens in", "2d 0h", "5:00 PM – 4:00 PM"})
	check(t, "Fri 6p", ct(2026, 11, 6, 18, 0), "CME", want{Closed, "Opens in", "1d 23h", ""})
	check(t, "Sun 5p", ct(2026, 11, 8, 17, 0), "CME", want{Open, "Break in", "23h 00m", ""})
}

func TestAsiaLunchBreaks(t *testing.T) {
	now := ct(2026, 11, 2, 20, 45) // = Tue 11:45 Tokyo, 10:45 Shanghai/Hong Kong
	check(t, "lunch", now, "TKY", want{Break, "Resumes in", "45m", ""})
	check(t, "lunch", now, "SHA", want{Open, "Break in", "45m", ""})
	check(t, "lunch", now, "HKG", want{Open, "Break in", "1h 15m", ""})
}

func TestEuropeUSDSTMismatchWeek(t *testing.T) {
	// Europe left summer time Oct 25; the US not until Nov 1, so London opens 3:00 AM Chicago.
	now := ct(2026, 10, 26, 2, 0)
	check(t, "Oct 26", now, "LON", want{Closed, "Opens in", "1h 00m", "3:00 AM – 11:30 AM"})
	check(t, "Oct 26", now, "FRA", want{Closed, "Opens in", "1h 00m", "3:00 AM – 11:30 AM"})
}

func TestSydneyWeekend(t *testing.T) {
	check(t, "Sat", ct(2026, 11, 7, 12, 0), "SYD", want{Closed, "Opens in", "1d 5h", "5:00 PM – 11:00 PM"})
	check(t, "Sat", ct(2026, 11, 7, 12, 0), "BOM", want{Closed, "Opens in", "1d 9h", ""})
}

func TestFmtDur(t *testing.T) {
	cases := map[time.Duration]string{
		0: "0m", 59 * time.Second: "1m", 61 * time.Minute: "1h 01m",
		26 * time.Hour: "1d 2h", -5 * time.Minute: "0m",
	}
	for d, w := range cases {
		if g := FmtDur(d); g != w {
			t.Errorf("FmtDur(%v) = %q, want %q", d, g, w)
		}
	}
}

func TestHoursText(t *testing.T) {
	if g := mkt("TKY").HoursText(true); g != "TSE · Tokyo time 9:00 AM – 3:30 PM · lunch 11:30 AM – 12:30 PM" {
		t.Errorf("TKY: %q", g)
	}
	if g := mkt("CME").HoursText(false); g != "CME Globex · Chicago time 17:00 – 16:00 next day, Sun–Fri · daily break 16:00 – 17:00" {
		t.Errorf("CME: %q", g)
	}
}
