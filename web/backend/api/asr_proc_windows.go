//go:build windows

package api

import "golang.org/x/sys/windows"

// processAlive reports whether pid refers to a live process.
//
// On Windows os.FindProcess always succeeds (it just wraps the PID), so we open
// a real handle instead: a released PID fails to open, and a live one opens.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)

	// A handle can open for a zombie; check the exit code explicitly.
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	const stillActive = 259 // STILL_ACTIVE
	return code == stillActive
}
