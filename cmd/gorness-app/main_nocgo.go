//go:build !darwin || !cgo

package main

import (
	"fmt"
	"os"
	"os/exec"
)

func main() {
	baseURL, _, err := ensureDesktopDaemon()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Gorness failed to start daemon: %v\n", err)
		os.Exit(1)
	}
	_ = exec.Command("/usr/bin/open", baseURL+"/").Start()
}
