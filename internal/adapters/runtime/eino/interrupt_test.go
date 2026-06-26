package eino

import (
	"os"
	"testing"

	runtimeport "fkteams/internal/ports/runtime"
)

func TestMain(m *testing.M) {
	restore := runtimeport.RegisterInterruptRuntime(NewInterruptRuntime())
	code := m.Run()
	restore()
	os.Exit(code)
}
