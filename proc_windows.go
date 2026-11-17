//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// cegah jendela konsol hitam muncul saat memanggil ccusage/npx
func hideWindow(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
