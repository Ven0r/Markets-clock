@echo off
rem Builds MarketClock.exe (needs Go 1.24+ from https://go.dev/dl/)
cd /d "%~dp0"
go test ./... || exit /b 1
go build -trimpath -ldflags "-s -w -H=windowsgui" -o MarketClock.exe .
if errorlevel 1 exit /b 1
echo Built MarketClock.exe
