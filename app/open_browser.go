package main

import (
	"os/exec"
	"runtime"
)

// openBrowser opens url in the user's default browser without waiting for it.
// Used by the web UI launcher, the SPA server and the DNS setup screen.
func openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default: // linux and others
		cmd = "xdg-open"
		args = []string{url}
	}

	return exec.Command(cmd, args...).Start()
}
