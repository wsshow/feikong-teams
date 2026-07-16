package scriptexec

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestCombinedOutputLimitsAndMarksOutput(t *testing.T) {
	command := exec.Command(os.Args[0], "-test.run=TestCombinedOutputHelper")
	command.Env = append(os.Environ(), "FKTEAMS_OUTPUT_HELPER=1")
	output, truncated, err := CombinedOutput(command, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated || String(output, truncated) != "1234\n[output truncated]" {
		t.Fatalf("output = %q, truncated = %v", String(output, truncated), truncated)
	}
}

func TestCombinedOutputAtExactLimitIsNotTruncated(t *testing.T) {
	command := exec.Command(os.Args[0], "-test.run=TestCombinedOutputHelper")
	command.Env = append(os.Environ(), "FKTEAMS_OUTPUT_HELPER=1")
	output, truncated, err := CombinedOutput(command, 10)
	if err != nil {
		t.Fatal(err)
	}
	if truncated || string(output) != "1234512345" {
		t.Fatalf("output = %q, truncated = %v", output, truncated)
	}
}

func TestCombinedOutputHelper(t *testing.T) {
	if os.Getenv("FKTEAMS_OUTPUT_HELPER") != "1" {
		return
	}
	fmt.Fprint(os.Stdout, strings.Repeat("12345", 2))
	os.Exit(0)
}

func TestStringDoesNotSplitUTF8(t *testing.T) {
	if got := String([]byte("123\xe4"), true); got != "123\n[output truncated]" {
		t.Fatalf("String() = %q", got)
	}
}
