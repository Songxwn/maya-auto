//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"
)

func openAppWindow(url string) (*exec.Cmd, error) {
	edge := findEdge()
	if edge == "" {
		return nil, openBrowser(url)
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	profile := filepath.Join(dir, "maya-auto", "webview")
	if err := os.MkdirAll(profile, 0o755); err != nil {
		return nil, err
	}
	cmd := exec.Command(edge,
		"--app="+url,
		"--user-data-dir="+profile,
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-sync",
		"--disable-extensions",
		"--window-size=1440,920",
	)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func openBrowser(url string) error {
	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}

func findEdge() string {
	roots := []string{
		os.Getenv("ProgramFiles(x86)"),
		os.Getenv("ProgramFiles"),
		os.Getenv("LocalAppData"),
	}
	for _, root := range roots {
		if root == "" {
			continue
		}
		path := filepath.Join(root, "Microsoft", "Edge", "Application", "msedge.exe")
		if st, err := os.Stat(path); err == nil && !st.IsDir() {
			return path
		}
	}
	return ""
}

func messageBox(title, text string) {
	user32 := syscall.NewLazyDLL("user32.dll")
	proc := user32.NewProc("MessageBoxW")
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(text)
	_, _, _ = proc.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0x10)
}
