package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Settings mirror the Pine inputs, plus a few desktop-only options.
// Saved as JSON in %APPDATA%\MarketClock\settings.json.
type Settings struct {
	TZ         string          `json:"timeZone"` // "" = follow Windows
	Use12h     bool            `json:"use12h"`
	Markets    map[string]bool `json:"markets"`
	ShowExch   bool            `json:"showExchange"`
	ShowNext   bool            `json:"showCountdown"`
	ShowHours  bool            `json:"showSessionHours"`
	ShowLabels bool            `json:"showColumnLabels"`
	ShowFooter bool            `json:"showFooter"`
	Theme      string          `json:"theme"` // Dark, Light, System
	Size       string          `json:"textSize"`
	Opacity    int             `json:"opacity"`
	Layer      string          `json:"layer"` // top, normal, bottom
	Locked     bool            `json:"lockPosition"`
	AllDesks   bool            `json:"showOnAllDesktops"`
	DesktopID  string          `json:"homeDesktop"` // virtual desktop it lives on
	HasPos     bool            `json:"hasPosition"`
	X          int             `json:"x"`
	Y          int             `json:"y"`
}

func defaultSettings() *Settings {
	s := &Settings{
		Use12h: true, Markets: map[string]bool{},
		ShowExch: true, ShowNext: true, ShowHours: true, ShowLabels: true, ShowFooter: true,
		Theme: "Dark", Size: "Small", Opacity: 100, Layer: "normal",
	}
	for _, m := range Markets {
		s.Markets[m.Key] = m.DefaultOn
	}
	return s
}

func settingsPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "MarketClock", "settings.json")
}

func loadSettings() *Settings {
	s := defaultSettings()
	if b, err := os.ReadFile(settingsPath()); err == nil {
		_ = json.Unmarshal(b, s)
	}
	for _, m := range Markets { // markets added in later versions default sensibly
		if _, ok := s.Markets[m.Key]; !ok {
			s.Markets[m.Key] = m.DefaultOn
		}
	}
	if s.Opacity < 30 || s.Opacity > 100 {
		s.Opacity = 100
	}
	return s
}

func (s *Settings) save() {
	p := settingsPath()
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	if b, err := json.MarshalIndent(s, "", "  "); err == nil {
		tmp := p + ".tmp"
		if os.WriteFile(tmp, b, 0o644) == nil {
			_ = os.Rename(tmp, p)
		}
	}
}
