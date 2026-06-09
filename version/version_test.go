package version

import "testing"

func TestGetAndString(t *testing.T) {
	info := Get()
	if info.Version != version {
		t.Fatalf("Version = %q, want %q", info.Version, version)
	}
	if info.BuildTime != buildTime {
		t.Fatalf("BuildTime = %q, want %q", info.BuildTime, buildTime)
	}
	if got := info.String(); got != version+" ("+buildTime+")" {
		t.Fatalf("String() = %q", got)
	}
}
