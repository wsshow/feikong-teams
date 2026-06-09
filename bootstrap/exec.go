package bootstrap

import (
	"os"
	"os/exec"
)

var (
	lookPath = exec.LookPath

	combinedOutput = func(name string, args ...string) ([]byte, error) {
		return exec.Command(name, args...).CombinedOutput()
	}

	runCommand = func(env []string, name string, args ...string) error {
		cmd := exec.Command(name, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if env != nil {
			cmd.Env = env
		}
		return cmd.Run()
	}
)
