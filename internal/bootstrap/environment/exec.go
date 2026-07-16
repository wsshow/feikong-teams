package environment

import (
	"fmt"
	"os"
	"os/exec"

	"fkteams/internal/runtime/executil"
)

const maxVersionOutputBytes int64 = 64 << 10

var (
	lookPath = exec.LookPath

	combinedOutput = func(name string, args ...string) ([]byte, error) {
		output, truncated, err := executil.CombinedOutput(exec.Command(name, args...), maxVersionOutputBytes)
		if err != nil {
			return output, err
		}
		if truncated {
			return output, fmt.Errorf("command output exceeds %d bytes", maxVersionOutputBytes)
		}
		return output, nil
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
