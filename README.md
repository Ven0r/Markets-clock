# Market Clock (desktop widget)

A small desktop widget for Windows that shows whether the major world stock and futures markets are
open, on a break, or closed, with a countdown to the next change and each session's hours in your
own time zone. It is a native Windows port of the Market Clock TradingView indicator.

- One ~2.6 MB `.exe`. No installer, no runtime, no browser engine.
- Wakes once a minute to redraw, otherwise idle.
- Built-in time zone database, so daylight saving changes are handled for every exchange.
- Exchange holidays are **not** included (same as the Pine script), so on a holiday a market may show
  as open when it is actually shut.

## Download

Get `MarketClock.exe` from the [latest release](https://github.com/Ven0r/Markets-clock/releases/latest).
It's a single file with nothing to install: put it somewhere permanent (for example
`C:\Tools\MarketClock\`) and double-click it. If Windows warns you about it, see
[If Windows warns you](#if-windows-warns-you).

## Who it's for

- **Traders and investors** who want to see at a glance which markets are live while they work, without
  opening TradingView or a browser.
- **Futures traders** who follow CME Globex overnight sessions alongside the cash equity markets.
- **Anyone who trades or works across time zones** and wants each exchange's hours converted to their
  local time.
- **TradingView Market Clock users** who want the same view as a desktop app.

It is informational only. It does not connect to a broker or place trades.

## What it shows

Each enabled market gets one row with its exchange name, its local time, its status (OPEN, BREAK or
CLOSED), a countdown to the next change, and its session hours. The title bar shows how many enabled
markets are open right now (for example "4 / 9 OPEN").

Markets included: Sydney (ASX), Tokyo (TSE), Hong Kong (HKEX), Shanghai (SSE), Mumbai (NSE),
Frankfurt (Xetra), Paris (Euronext, off by default), London (LSE), New York (NYSE) and Chicago
(CME Globex futures, which run overnight Sunday to Friday).

## Platforms

- **Windows 10 and Windows 11.** Windows 11 adds rounded corners and a themed border; the rest works the
  same on both.
- **Not macOS or Linux.** The code is Windows-only. Building it on those systems produces a program that
  only prints a message saying it is a Windows app. On Linux, the Windows exe does run under
  [Wine](https://www.winehq.org/), but without the one-desktop feature or Start with Windows.
- Runs on the Windows desktop itself. It is not a WSL/Linux GUI app.

## Using it

Double-click `MarketClock.exe`. It opens in the top-right corner of your screen.

- **Move it:** drag it anywhere.
- **Options:** right-click the widget (or the tray icon). Everything from the Pine inputs is there:
  markets, columns, your time zone, 12/24-hour, theme, text size. Plus desktop-only options:
  opacity, always on top / normal / behind other windows, lock position, start with Windows,
  show on all desktops.
- **Hover a row:** the footer shows that exchange's hours in its own local time (lunch breaks too).
- **Hide / show:** left-click the tray icon. **Quit:** right-click → Exit Market Clock.

### Virtual desktops

The widget lives on one virtual desktop: the one you start it on. It hides itself on your other
desktops and reappears when you switch back. (It watches which desktop is active, so it never
needs a taskbar button and stays out of Alt+Tab.)

- **Move it to another desktop:** switch to that desktop and click the tray icon (or right-click
  the tray icon → Move the widget to this desktop).
- **Which desktop is it on?** Right-click → Window shows "Lives on: Desktop N".
- **Start with Windows:** it comes back on the same desktop it lived on before.
- **Want it on every desktop instead?** Right-click → Window → Show on all desktops.

Settings are saved to `%APPDATA%\MarketClock\settings.json`.

Tip: put the exe somewhere permanent (e.g. `C:\Tools\MarketClock\`) *before* turning on
"Start with Windows", since that setting points at the exe's current location.

## If Windows warns you

Market Clock is new and isn't code-signed, so Windows has no reputation for it and may warn you the
first time:

- **"Windows protected your PC"** (SmartScreen): click **More info → Run anyway**.
- **Microsoft Defender reports a threat, or the exe disappears:** Defender's machine-learning scanner
  sometimes flags unsigned programs built with Go. To allow it, open **Windows Security → Virus &
  threat protection → Protection history**, select the Market Clock entry, then **Actions → Restore**
  (or **Allow on device**). Then add an exclusion so later scans leave it alone: **Virus & threat
  protection → Manage settings → Exclusions → Add or remove exclusions → Add an exclusion → File**,
  and pick `MarketClock.exe`.
- **Smart App Control blocks it:** Smart App Control can't make an exception for a single app, so
  the widget can't run while it's turned on.

The app makes no network connections. It only reads and writes its own settings file and, if you
turn the option on, its Start with Windows entry. If you'd rather not run a downloaded exe, build it
yourself from this source (below).

## Building from source

Requirements: Windows, and Go 1.24 or newer from https://go.dev/dl/. No other dependencies.

1. Download or clone this repository.
2. In the folder, run `build.bat`. It runs the tests, then writes `MarketClock.exe`.

The icon, DPI-awareness manifest and version info are already compiled into
`rsrc_windows_amd64.syso`, which `go build` picks up automatically. To change the icon, edit
`tools/genicon/main.go`, run `go run ./tools/genicon`, then recompile the resources with `windres`
(from MinGW-w64): `cd res && windres -O coff -o ../rsrc_windows_amd64.syso app.rc`.

`go test ./...` runs the session-logic tests (weekends, lunch breaks, CME overnight sessions,
daylight-saving mismatch weeks).

## Files

| File | What it is |
| --- | --- |
| `sessions.go` | Market hours and the open/break/closed logic, ported from the Pine script |
| `sessions_test.go` | Tests for that logic |
| `settings.go` | Saved settings |
| `app_windows.go` | Window, menu, tray icon, timers |
| `paint_windows.go` | Drawing the widget |
| `win32_windows.go` | Windows API bindings |
| `tz_windows.go` | Maps your Windows time zone to an IANA zone |
| `vdesk_windows.go` | Follows the active virtual desktop |
| `main_other.go` | Stub for non-Windows builds (prints a message and exits) |

## Changing market hours

Edit the `Markets` list at the top of `sessions.go`. Times are minutes after midnight in the
exchange's own time zone (9:30 AM = 570). Rebuild with `build.bat`.

## Testing a different time

Set `MARKETCLOCK_NOW` before launching to see how the widget looks at another moment, e.g. in
PowerShell: `$env:MARKETCLOCK_NOW="2026-11-02T20:45:00-06:00"; .\MarketClock.exe`
