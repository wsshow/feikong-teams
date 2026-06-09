package bootstrap

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

type fakeInitializer struct {
	name  string
	err   error
	calls *[]string
}

func (f *fakeInitializer) Name() string { return f.name }

func (f *fakeInitializer) Run() error {
	*f.calls = append(*f.calls, "run:"+f.name)
	return f.err
}

type fakeMirrorInitializer struct {
	*fakeInitializer
	mirrors *[]bool
}

func (f *fakeMirrorInitializer) ConfigureMirror(mirror bool) {
	*f.mirrors = append(*f.mirrors, mirror)
}

type commandCall struct {
	env  []string
	name string
	args []string
}

func withInitializers(t *testing.T, values []Initializer) {
	t.Helper()

	original := initializers
	initializers = values
	t.Cleanup(func() {
		initializers = original
	})
}

func withCommandHooks(
	t *testing.T,
	look func(string) (string, error),
	output func(string, ...string) ([]byte, error),
	run func([]string, string, ...string) error,
) {
	t.Helper()

	originalLookPath := lookPath
	originalCombinedOutput := combinedOutput
	originalRunCommand := runCommand
	lookPath = look
	combinedOutput = output
	runCommand = run
	t.Cleanup(func() {
		lookPath = originalLookPath
		combinedOutput = originalCombinedOutput
		runCommand = originalRunCommand
	})
}

func bootstrapHome(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("FEIKONG_PROXY_URL", "")
	return home
}

func uvConfigPath(home string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(home, "AppData", "Roaming", "uv", "uv.toml")
	}
	return filepath.Join(home, ".config", "uv", "uv.toml")
}

func bunConfigPath(home string) string {
	return filepath.Join(home, ".bunfig.toml")
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("expected %q to contain %q", got, want)
	}
}

func TestNamesUsesRegisteredInitializers(t *testing.T) {
	var calls []string
	withInitializers(t, []Initializer{
		&fakeInitializer{name: "uv", calls: &calls},
		&fakeInitializer{name: "bun", calls: &calls},
	})

	got := Names()
	want := []string{"uv", "bun"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Names() = %v, want %v", got, want)
	}
}

func TestRunWithDefaultsToAllAndConfiguresMirror(t *testing.T) {
	var calls []string
	var mirrors []bool
	withInitializers(t, []Initializer{
		&fakeMirrorInitializer{
			fakeInitializer: &fakeInitializer{name: "uv", calls: &calls},
			mirrors:         &mirrors,
		},
		&fakeInitializer{name: "bun", calls: &calls},
	})

	RunWith(nil, true)

	wantCalls := []string{"run:uv", "run:bun"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls = %v, want %v", calls, wantCalls)
	}
	wantMirrors := []bool{true}
	if !reflect.DeepEqual(mirrors, wantMirrors) {
		t.Fatalf("mirrors = %v, want %v", mirrors, wantMirrors)
	}
}

func TestRunWithSkipsUnknownAndContinuesAfterError(t *testing.T) {
	var calls []string
	withInitializers(t, []Initializer{
		&fakeInitializer{name: "bad", calls: &calls, err: errors.New("boom")},
		&fakeInitializer{name: "ok", calls: &calls},
	})

	RunWith([]string{"missing", "bad", "ok"}, false)

	want := []string{"run:bad", "run:ok"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestAppendProxyEnv(t *testing.T) {
	base := []string{"EXISTING=value"}
	t.Setenv("FEIKONG_PROXY_URL", "")
	if got := appendProxyEnv(base); !reflect.DeepEqual(got, base) {
		t.Fatalf("appendProxyEnv without proxy = %v, want %v", got, base)
	}

	t.Setenv("FEIKONG_PROXY_URL", "http://127.0.0.1:7890")
	got := appendProxyEnv(base)
	for _, want := range []string{
		"HTTP_PROXY=http://127.0.0.1:7890",
		"HTTPS_PROXY=http://127.0.0.1:7890",
		"http_proxy=http://127.0.0.1:7890",
		"https_proxy=http://127.0.0.1:7890",
	} {
		if !containsString(got, want) {
			t.Fatalf("appendProxyEnv() = %v, missing %q", got, want)
		}
	}
}

func TestInitializerNames(t *testing.T) {
	if got := (&uvInitializer{}).Name(); got != "uv" {
		t.Fatalf("uv name = %q, want uv", got)
	}
	if got := (&bunInitializer{}).Name(); got != "bun" {
		t.Fatalf("bun name = %q, want bun", got)
	}
}

func TestConfigureMirrorSkipsWhenDisabled(t *testing.T) {
	home := bootstrapHome(t)

	(&uvInitializer{}).ConfigureMirror(false)
	(&bunInitializer{}).ConfigureMirror(false)

	if _, err := os.Stat(uvConfigPath(home)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("uv config stat error = %v, want not exist", err)
	}
	if _, err := os.Stat(bunConfigPath(home)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("bun config stat error = %v, want not exist", err)
	}
}

func TestUVConfigureMirrorWritesAndSkipsExisting(t *testing.T) {
	home := bootstrapHome(t)
	path := uvConfigPath(home)

	(&uvInitializer{}).ConfigureMirror(true)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read uv config: %v", err)
	}
	assertContains(t, string(data), "mirrors.aliyun.com")

	(&uvInitializer{}).ConfigureMirror(true)
	again, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read uv config again: %v", err)
	}
	if string(again) != string(data) {
		t.Fatalf("uv config changed on second configure")
	}
}

func TestBunConfigureMirrorWritesAndSkipsExisting(t *testing.T) {
	home := bootstrapHome(t)
	path := bunConfigPath(home)

	(&bunInitializer{}).ConfigureMirror(true)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read bun config: %v", err)
	}
	assertContains(t, string(data), "registry.npmmirror.com")

	(&bunInitializer{}).ConfigureMirror(true)
	again, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read bun config again: %v", err)
	}
	if string(again) != string(data) {
		t.Fatalf("bun config changed on second configure")
	}
}

func TestUVRunInstallsWhenCommandMissing(t *testing.T) {
	t.Setenv("FEIKONG_PROXY_URL", "http://proxy.example")
	var calls []commandCall
	withCommandHooks(t,
		func(string) (string, error) {
			return "", errors.New("missing")
		},
		func(string, ...string) ([]byte, error) {
			t.Fatal("combinedOutput should not be called")
			return nil, nil
		},
		func(env []string, name string, args ...string) error {
			calls = append(calls, commandCall{env: append([]string(nil), env...), name: name, args: append([]string(nil), args...)})
			return nil
		},
	)

	if err := (&uvInitializer{}).Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("command calls = %v, want one call", calls)
	}
	if runtime.GOOS == "windows" {
		if calls[0].name != "powershell" {
			t.Fatalf("install command = %q, want powershell", calls[0].name)
		}
	} else if calls[0].name != "sh" {
		t.Fatalf("install command = %q, want sh", calls[0].name)
	}
	if !containsString(calls[0].env, "HTTP_PROXY=http://proxy.example") {
		t.Fatalf("install env = %v, missing proxy", calls[0].env)
	}
}

func TestUVRunUpgradesExistingCommand(t *testing.T) {
	var calls []commandCall
	var versionCalls int
	withCommandHooks(t,
		func(name string) (string, error) {
			if name != "uv" {
				t.Fatalf("lookPath name = %q, want uv", name)
			}
			return "/fake/uv", nil
		},
		func(name string, args ...string) ([]byte, error) {
			versionCalls++
			if name != "/fake/uv" || !reflect.DeepEqual(args, []string{"--version"}) {
				t.Fatalf("combinedOutput(%q, %v), want /fake/uv --version", name, args)
			}
			return []byte("uv 1.0.0\n"), nil
		},
		func(env []string, name string, args ...string) error {
			calls = append(calls, commandCall{env: env, name: name, args: append([]string(nil), args...)})
			return nil
		},
	)

	if err := (&uvInitializer{}).Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if versionCalls != 2 {
		t.Fatalf("version calls = %d, want 2", versionCalls)
	}
	if len(calls) != 1 || calls[0].name != "/fake/uv" || !reflect.DeepEqual(calls[0].args, []string{"self", "update"}) {
		t.Fatalf("upgrade calls = %v, want /fake/uv self update", calls)
	}
}

func TestBunRunUpgradesExistingCommand(t *testing.T) {
	var calls []commandCall
	var versionCalls int
	withCommandHooks(t,
		func(name string) (string, error) {
			if name != "bun" {
				t.Fatalf("lookPath name = %q, want bun", name)
			}
			return "/fake/bun", nil
		},
		func(name string, args ...string) ([]byte, error) {
			versionCalls++
			if name != "/fake/bun" || !reflect.DeepEqual(args, []string{"--version"}) {
				t.Fatalf("combinedOutput(%q, %v), want /fake/bun --version", name, args)
			}
			return []byte("1.0.0\n"), nil
		},
		func(env []string, name string, args ...string) error {
			calls = append(calls, commandCall{env: env, name: name, args: append([]string(nil), args...)})
			return nil
		},
	)

	if err := (&bunInitializer{}).Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if versionCalls != 2 {
		t.Fatalf("version calls = %d, want 2", versionCalls)
	}
	if len(calls) != 1 || calls[0].name != "/fake/bun" || !reflect.DeepEqual(calls[0].args, []string{"upgrade"}) {
		t.Fatalf("upgrade calls = %v, want /fake/bun upgrade", calls)
	}
}

func TestBunUpgradeReturnsMissingCommandError(t *testing.T) {
	withCommandHooks(t,
		func(string) (string, error) {
			return "", errors.New("missing")
		},
		combinedOutput,
		runCommand,
	)

	err := (&bunInitializer{}).upgrade()
	if err == nil {
		t.Fatal("upgrade() error = nil, want error")
	}
	assertContains(t, err.Error(), "bun not found")
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
