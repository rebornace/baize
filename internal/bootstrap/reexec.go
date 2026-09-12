package bootstrap

import (
	"os"
	"os/exec"
	"runtime"
	"syscall"
)

// Reexec replaces the current process with a fresh instance using the same
// executable and arguments. Tests may replace this variable.
var Reexec = reexec

func reexec() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	args := os.Args
	env := os.Environ()

	if runtime.GOOS == "windows" {
		cmd := exec.Command(exe, args[1:]...)
		cmd.Env = env
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		if err := cmd.Start(); err != nil {
			return err
		}
		os.Exit(0)
	}
	return syscall.Exec(exe, args, env)
}
