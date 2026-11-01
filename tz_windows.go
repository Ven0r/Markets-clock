//go:build windows

package main

import (
	"syscall"
	"time"
	"unsafe"
)

// Windows time zone key → IANA name, for "Match Windows". Covers the common
// zones; anything else falls back to Windows' own local time with label "Local".
var winToIANA = map[string]string{
	"Central Standard Time":          "America/Chicago",
	"Eastern Standard Time":          "America/New_York",
	"Mountain Standard Time":         "America/Denver",
	"US Mountain Standard Time":      "America/Phoenix",
	"Pacific Standard Time":          "America/Los_Angeles",
	"Alaskan Standard Time":          "America/Anchorage",
	"Hawaiian Standard Time":         "Pacific/Honolulu",
	"Atlantic Standard Time":         "America/Halifax",
	"Newfoundland Standard Time":     "America/St_Johns",
	"Canada Central Standard Time":   "America/Regina",
	"Central America Standard Time":  "America/Guatemala",
	"Central Standard Time (Mexico)": "America/Mexico_City",
	"SA Pacific Standard Time":       "America/Bogota",
	"E. South America Standard Time": "America/Sao_Paulo",
	"Argentina Standard Time":        "America/Buenos_Aires",
	"UTC":                            "UTC",
	"GMT Standard Time":              "Europe/London",
	"W. Europe Standard Time":        "Europe/Berlin",
	"Romance Standard Time":          "Europe/Paris",
	"Central Europe Standard Time":   "Europe/Budapest",
	"Central European Standard Time": "Europe/Warsaw",
	"GTB Standard Time":              "Europe/Bucharest",
	"FLE Standard Time":              "Europe/Kiev",
	"Turkey Standard Time":           "Europe/Istanbul",
	"Russian Standard Time":          "Europe/Moscow",
	"South Africa Standard Time":     "Africa/Johannesburg",
	"Israel Standard Time":           "Asia/Jerusalem",
	"Arabian Standard Time":          "Asia/Dubai",
	"Arab Standard Time":             "Asia/Riyadh",
	"India Standard Time":            "Asia/Kolkata",
	"SE Asia Standard Time":          "Asia/Bangkok",
	"Singapore Standard Time":        "Asia/Singapore",
	"China Standard Time":            "Asia/Shanghai",
	"Taipei Standard Time":           "Asia/Taipei",
	"Tokyo Standard Time":            "Asia/Tokyo",
	"Korea Standard Time":            "Asia/Seoul",
	"W. Australia Standard Time":     "Australia/Perth",
	"E. Australia Standard Time":     "Australia/Brisbane",
	"AUS Eastern Standard Time":      "Australia/Sydney",
	"New Zealand Standard Time":      "Pacific/Auckland",
}

// systemZone returns the zone Windows is set to and a city label for it.
func systemZone() (*time.Location, string) {
	var tzi DYNAMIC_TIME_ZONE_INFORMATION
	r, _, _ := pGetDynamicTimeZoneInformation.Call(uintptr(unsafe.Pointer(&tzi)))
	if r != 0xFFFFFFFF {
		key := syscall.UTF16ToString(tzi.TimeZoneKeyName[:])
		if iana, ok := winToIANA[key]; ok {
			return loadZone(iana), CityOf(iana)
		}
	}
	return time.Local, "Local"
}
