//go:build !windows

package main

import "os/exec"

func openAppWindow(url string) (*exec.Cmd, error) {
	return nil, openBrowser(url)
}

func openBrowser(url string) error {
	cmd := exec.Command("xdg-open", url)
	return cmd.Start()
}

func messageBox(_, text string) {
	println(text)
}
