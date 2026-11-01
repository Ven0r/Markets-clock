//go:build !windows

package main

import "fmt"

func main() { fmt.Println("Market Clock is a Windows app. Build with GOOS=windows.") }
