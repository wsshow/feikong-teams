package uv

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func newTestUVTools(t *testing.T) *UVTools {
	t.Helper()

	root := t.TempDir()
	envDir := filepath.Join(root, "env")
	workDir := filepath.Join(root, "work")
	if err := os.MkdirAll(envDir, 0755); err != nil {
		t.Fatalf("create env dir: %v", err)
	}
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatalf("create work dir: %v", err)
	}
	return &UVTools{
		envDir:   envDir,
		workDir:  workDir,
		venvPath: filepath.Join(envDir, ".venv"),
		uvPath:   writeFakeCommand(t, root, "uv", fakeUVScript()),
	}
}

func writeFakeCommand(t *testing.T, dir, name, body string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if runtime.GOOS == "windows" {
		path += ".bat"
	}
	if err := os.WriteFile(path, []byte(body), 0755); err != nil {
		t.Fatalf("write fake command: %v", err)
	}
	return path
}

func fakeUVScript() string {
	if runtime.GOOS == "windows" {
		return "@echo off\r\necho []\r\n"
	}
	return `#!/bin/sh
echo "$@" >> "$UV_TEST_LOG"
if [ "$1" = "--version" ]; then
  echo "uv 0.1.0"
  exit 0
fi
if [ "$1" = "pip" ] && [ "$2" = "list" ]; then
  echo '[{"name":"requests","version":"2.32.0"},{"name":"pytest","version":"8.0.0"}]'
  exit 0
fi
echo "uv ok"
`
}

func createVenv(t *testing.T, ut *UVTools) {
	t.Helper()

	pythonPath := ut.getPythonPath()
	if err := os.MkdirAll(filepath.Dir(pythonPath), 0755); err != nil {
		t.Fatalf("create venv bin: %v", err)
	}
	body := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo \"Python 3.12.0\"; exit 0; fi\necho python-ok\n"
	if runtime.GOOS == "windows" {
		body = "@echo off\r\necho Python 3.12.0\r\n"
	}
	if err := os.WriteFile(pythonPath, []byte(body), 0755); err != nil {
		t.Fatalf("write fake python: %v", err)
	}
}

func readLog(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	return string(data)
}

func TestNewUVToolsUsesPathAndCreatesDirs(t *testing.T) {
	root := t.TempDir()
	fakeUV := writeFakeCommand(t, root, "uv", fakeUVScript())
	t.Setenv("PATH", root)

	ut, err := NewUVTools(filepath.Join(root, "env"), filepath.Join(root, "work"))
	if err != nil {
		t.Fatalf("NewUVTools returned error: %v", err)
	}
	if ut.uvPath != fakeUV {
		t.Fatalf("uvPath = %q, want %q", ut.uvPath, fakeUV)
	}
	if _, err := os.Stat(ut.envDir); err != nil {
		t.Fatalf("env dir not created: %v", err)
	}
	if _, err := os.Stat(ut.workDir); err != nil {
		t.Fatalf("work dir not created: %v", err)
	}
}

func TestUVGetTools(t *testing.T) {
	var nilTools *UVTools
	if tools, err := nilTools.GetTools(); err == nil || tools != nil {
		t.Fatalf("nil GetTools tools=%#v err=%v, want error", tools, err)
	}

	tools, err := newTestUVTools(t).GetTools()
	if err != nil {
		t.Fatalf("GetTools returned error: %v", err)
	}
	if len(tools) != 10 {
		t.Fatalf("tool count = %d, want 10", len(tools))
	}
	info, err := tools[0].Info(context.Background())
	if err != nil {
		t.Fatalf("tool info returned error: %v", err)
	}
	if info.Name != "uv_init_env" {
		t.Fatalf("first tool = %q, want uv_init_env", info.Name)
	}
}

func TestUVInitEnvExisting(t *testing.T) {
	ut := newTestUVTools(t)
	createVenv(t, ut)

	resp, err := ut.InitEnv(context.Background(), &InitEnvRequest{})
	if err != nil {
		t.Fatalf("InitEnv returned error: %v", err)
	}
	if !resp.Success || !strings.Contains(resp.Message, "虚拟环境已存在") || resp.PythonPath != ut.getPythonPath() {
		t.Fatalf("InitEnv existing response = %#v", resp)
	}
}

func TestUVPackageCommands(t *testing.T) {
	ut := newTestUVTools(t)
	createVenv(t, ut)
	logPath := filepath.Join(t.TempDir(), "uv.log")
	t.Setenv("UV_TEST_LOG", logPath)

	installResp, err := ut.InstallPackage(context.Background(), &InstallPackageRequest{Packages: []string{"requests", "pytest"}, Upgrade: true})
	if err != nil {
		t.Fatalf("InstallPackage returned error: %v", err)
	}
	if !installResp.Success || strings.Join(installResp.Installed, ",") != "requests,pytest" {
		t.Fatalf("install response = %#v", installResp)
	}

	removeResp, err := ut.RemovePackage(context.Background(), &RemovePackageRequest{Packages: []string{"pytest"}})
	if err != nil {
		t.Fatalf("RemovePackage returned error: %v", err)
	}
	if !removeResp.Success || strings.Join(removeResp.Removed, ",") != "pytest" {
		t.Fatalf("remove response = %#v", removeResp)
	}

	log := readLog(t, logPath)
	for _, want := range []string{"pip install --upgrade requests pytest", "pip uninstall -y pytest"} {
		if !strings.Contains(log, want) {
			t.Fatalf("command log missing %q: %q", want, log)
		}
	}
}

func TestUVPackageValidationErrors(t *testing.T) {
	ut := newTestUVTools(t)

	if resp, err := ut.InstallPackage(context.Background(), &InstallPackageRequest{}); err != nil || resp.Success || resp.ErrorMessage == "" {
		t.Fatalf("empty install resp=%#v err=%v", resp, err)
	}
	if resp, err := ut.RemovePackage(context.Background(), &RemovePackageRequest{}); err != nil || resp.Success || resp.ErrorMessage == "" {
		t.Fatalf("empty remove resp=%#v err=%v", resp, err)
	}
	if resp, err := ut.InstallPackage(context.Background(), &InstallPackageRequest{Packages: []string{"requests"}}); err != nil || resp.Success || !strings.Contains(resp.ErrorMessage, "虚拟环境不存在") {
		t.Fatalf("missing venv install resp=%#v err=%v", resp, err)
	}
	if resp, err := ut.ListPackage(context.Background(), &ListPackageRequest{}); err != nil || resp.Success || !strings.Contains(resp.ErrorMessage, "虚拟环境不存在") {
		t.Fatalf("missing venv list resp=%#v err=%v", resp, err)
	}
}

func TestUVListPackage(t *testing.T) {
	ut := newTestUVTools(t)
	createVenv(t, ut)
	t.Setenv("UV_TEST_LOG", filepath.Join(t.TempDir(), "uv.log"))

	resp, err := ut.ListPackage(context.Background(), &ListPackageRequest{})
	if err != nil {
		t.Fatalf("ListPackage returned error: %v", err)
	}
	if !resp.Success || len(resp.Packages) != 2 || resp.Packages[0].Name != "requests" {
		t.Fatalf("list response = %#v", resp)
	}

	textResp, err := ut.ListPackage(context.Background(), &ListPackageRequest{Format: "text"})
	if err != nil {
		t.Fatalf("ListPackage text returned error: %v", err)
	}
	if !strings.Contains(textResp.Message, "requests==2.32.0") || !strings.Contains(textResp.Message, "pytest==8.0.0") {
		t.Fatalf("text list message = %q", textResp.Message)
	}
}

func TestUVCleanEnv(t *testing.T) {
	ut := newTestUVTools(t)

	resp, err := ut.CleanEnv(context.Background(), &CleanEnvRequest{})
	if err != nil {
		t.Fatalf("CleanEnv missing venv returned error: %v", err)
	}
	if !resp.Success || !strings.Contains(resp.Message, "无需清理") {
		t.Fatalf("missing venv clean response = %#v", resp)
	}

	createVenv(t, ut)
	resp, err = ut.CleanEnv(context.Background(), &CleanEnvRequest{})
	if err != nil {
		t.Fatalf("CleanEnv returned error: %v", err)
	}
	if !resp.Success {
		t.Fatalf("clean response = %#v", resp)
	}
	if _, err := os.Stat(ut.venvPath); !os.IsNotExist(err) {
		t.Fatalf("venv should be removed, stat err=%v", err)
	}
}

func TestUVCleanEnvKeepsVenvAndRemovesPackages(t *testing.T) {
	ut := newTestUVTools(t)
	createVenv(t, ut)
	logPath := filepath.Join(t.TempDir(), "uv.log")
	t.Setenv("UV_TEST_LOG", logPath)

	resp, err := ut.CleanEnv(context.Background(), &CleanEnvRequest{KeepVenv: true})
	if err != nil {
		t.Fatalf("CleanEnv keep venv returned error: %v", err)
	}
	if !resp.Success || !strings.Contains(resp.Message, "已移除 2 个包") {
		t.Fatalf("clean keep response = %#v", resp)
	}
	if _, err := os.Stat(ut.venvPath); err != nil {
		t.Fatalf("venv should be kept: %v", err)
	}
	log := readLog(t, logPath)
	for _, want := range []string{"pip list --format json", "pip uninstall -y requests pytest"} {
		if !strings.Contains(log, want) {
			t.Fatalf("command log missing %q: %q", want, log)
		}
	}
}

func TestUVRunAndFormatValidation(t *testing.T) {
	ut := newTestUVTools(t)

	if resp, err := ut.RunScript(context.Background(), &RunScriptRequest{}); err != nil || resp.Success || !strings.Contains(resp.ErrorMessage, "虚拟环境不存在") {
		t.Fatalf("RunScript missing venv resp=%#v err=%v", resp, err)
	}
	if resp, err := ut.RunCode(context.Background(), &RunCodeRequest{}); err != nil || resp.Success || !strings.Contains(resp.ErrorMessage, "code") {
		t.Fatalf("RunCode missing code resp=%#v err=%v", resp, err)
	}
	if resp, err := ut.CheckSyntax(context.Background(), &CheckSyntaxRequest{}); err != nil || resp.Valid || !strings.Contains(resp.ErrorMessage, "必须提供") {
		t.Fatalf("CheckSyntax missing input resp=%#v err=%v", resp, err)
	}
	if resp, err := ut.FormatCode(context.Background(), &FormatCodeRequest{}); err != nil || resp.Success || !strings.Contains(resp.ErrorMessage, "code") {
		t.Fatalf("FormatCode missing code resp=%#v err=%v", resp, err)
	}
}

func TestUVGetEnvInfoMissingVenv(t *testing.T) {
	ut := newTestUVTools(t)

	resp, err := ut.GetEnvInfo(context.Background(), &GetEnvInfoRequest{})
	if err != nil {
		t.Fatalf("GetEnvInfo returned error: %v", err)
	}
	if !resp.Success || resp.Exists || resp.VenvPath != ut.venvPath || !strings.Contains(resp.ErrorMessage, "虚拟环境不存在") {
		t.Fatalf("env info response = %#v", resp)
	}
}

func TestUVGetEnvInfoExistingVenv(t *testing.T) {
	ut := newTestUVTools(t)
	createVenv(t, ut)
	t.Setenv("UV_TEST_LOG", filepath.Join(t.TempDir(), "uv.log"))

	resp, err := ut.GetEnvInfo(context.Background(), &GetEnvInfoRequest{})
	if err != nil {
		t.Fatalf("GetEnvInfo returned error: %v", err)
	}
	if !resp.Success ||
		!resp.Exists ||
		resp.PythonPath != ut.getPythonPath() ||
		resp.PythonVersion != "Python 3.12.0" ||
		resp.UVVersion != "uv 0.1.0" ||
		resp.PackageCount != 2 {
		t.Fatalf("env info response = %#v", resp)
	}
}
