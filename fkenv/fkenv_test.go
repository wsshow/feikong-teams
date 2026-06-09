package fkenv

import "testing"

func TestGetReadsEnvironmentVariable(t *testing.T) {
	t.Setenv(AppDir, "/tmp/fkteams-test")

	if got := Get(AppDir); got != "/tmp/fkteams-test" {
		t.Fatalf("Get(AppDir) = %q, want configured value", got)
	}
	if got := Get("FEIKONG_UNKNOWN_TEST_KEY"); got != "" {
		t.Fatalf("Get(unknown) = %q, want empty", got)
	}
}
