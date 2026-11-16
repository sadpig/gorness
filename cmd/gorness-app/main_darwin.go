//go:build darwin && cgo

package main

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework WebKit

#include <stdlib.h>

void runCocoaDesktop(const char *baseURL, const char *errMsg);
*/
import "C"

import (
	"os/exec"
	"runtime"
	"unsafe"
)

var (
	activeDaemonBaseURL string
	activeVaultRoot     string
)

//export goDecideNavigation
func goDecideNavigation(cURL *C.char) C.int {
	target := C.GoString(cURL)
	inApp, ext := IsAllowedLoopbackNavigation(target, activeDaemonBaseURL)
	if inApp {
		return 1
	}
	if ext {
		return 2
	}
	return 0
}

//export goOnFolderChosen
func goOnFolderChosen(cPath *C.char) {
	folder := C.GoString(cPath)
	_ = registerProjectDirectory(activeDaemonBaseURL, folder)
}

//export goOpenVaultInFinder
func goOpenVaultInFinder() {
	if activeVaultRoot != "" {
		_ = exec.Command("/usr/bin/open", activeVaultRoot).Start()
	}
}

func main() {
	runtime.LockOSThread()
	baseURL, vaultRoot, err := ensureDesktopDaemon()
	activeDaemonBaseURL = baseURL
	activeVaultRoot = vaultRoot

	var cBase *C.char
	var cErr *C.char
	if err != nil {
		cBase = C.CString("")
		cErr = C.CString(err.Error())
	} else {
		cBase = C.CString(baseURL)
		cErr = C.CString("")
	}
	defer C.free(unsafe.Pointer(cBase))
	defer C.free(unsafe.Pointer(cErr))

	C.runCocoaDesktop(cBase, cErr)
}
