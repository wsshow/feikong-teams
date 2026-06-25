package bun

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func newTestBunTools(t *testing.T) *BunTools {
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
	return &BunTools{
		envDir:  envDir,
		workDir: workDir,
		bunPath: writeFakeCommand(t, root, "bun", fakeBunScript()),
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

func fakeBunScript() string {
	if runtime.GOOS == "windows" {
		return "@echo off\r\necho bun ok\r\n"
	}
	return `#!/bin/sh
echo "$@" >> "$BUN_TEST_LOG"
if [ "$1" = "--version" ]; then
  echo "1.2.3"
  exit 0
fi
if [ "$1" = "init" ]; then
  cat > package.json <<'JSON'
{"dependencies":{},"devDependencies":{}}
JSON
  echo "bun init ok"
  exit 0
fi
echo "bun ok"
`
}

func writePackageJSON(t *testing.T, bt *BunTools, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(bt.envDir, "package.json"), []byte(content), 0644); err != nil {
		t.Fatalf("write package.json: %v", err)
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

func TestNewBunToolsUsesPathAndCreatesDirs(t *testing.T) {
	root := t.TempDir()
	fakeBun := writeFakeCommand(t, root, "bun", fakeBunScript())
	t.Setenv("PATH", root)

	bt, err := NewBunTools(filepath.Join(root, "env"), filepath.Join(root, "work"))
	if err != nil {
		t.Fatalf("NewBunTools returned error: %v", err)
	}
	if bt.bunPath != fakeBun {
		t.Fatalf("bunPath = %q, want %q", bt.bunPath, fakeBun)
	}
	if _, err := os.Stat(bt.envDir); err != nil {
		t.Fatalf("env dir not created: %v", err)
	}
	if _, err := os.Stat(bt.workDir); err != nil {
		t.Fatalf("work dir not created: %v", err)
	}
}

func TestBunGetTools(t *testing.T) {
	var nilTools *BunTools
	if tools, err := nilTools.GetTools(); err == nil || tools != nil {
		t.Fatalf("nil GetTools tools=%#v err=%v, want error", tools, err)
	}

	tools, err := newTestBunTools(t).GetTools()
	if err != nil {
		t.Fatalf("GetTools returned error: %v", err)
	}
	if len(tools) != 7 {
		t.Fatalf("tool count = %d, want 7", len(tools))
	}
	info, err := tools[0].Info(context.Background())
	if err != nil {
		t.Fatalf("tool info returned error: %v", err)
	}
	if info.Name != "bun_init_env" {
		t.Fatalf("first tool = %q, want bun_init_env", info.Name)
	}
}

func TestBunInitEnvExistingAndForce(t *testing.T) {
	bt := newTestBunTools(t)
	writePackageJSON(t, bt, `{"dependencies":{}}`)
	logPath := filepath.Join(t.TempDir(), "bun.log")
	t.Setenv("BUN_TEST_LOG", logPath)

	resp, err := bt.InitEnv(context.Background(), &InitEnvRequest{})
	if err != nil {
		t.Fatalf("InitEnv returned error: %v", err)
	}
	if !resp.Success || !strings.Contains(resp.Message, "package.json 已存在") {
		t.Fatalf("InitEnv existing response = %#v", resp)
	}

	resp, err = bt.InitEnv(context.Background(), &InitEnvRequest{Force: true})
	if err != nil {
		t.Fatalf("InitEnv force returned error: %v", err)
	}
	if !resp.Success || !strings.Contains(resp.Message, "项目初始化成功") {
		t.Fatalf("InitEnv force response = %#v", resp)
	}
	if log := readLog(t, logPath); !strings.Contains(log, "init -y") {
		t.Fatalf("command log = %q, want init -y", log)
	}
}

func TestBunPackageCommands(t *testing.T) {
	bt := newTestBunTools(t)
	writePackageJSON(t, bt, `{"dependencies":{},"devDependencies":{}}`)
	logPath := filepath.Join(t.TempDir(), "bun.log")
	t.Setenv("BUN_TEST_LOG", logPath)

	installResp, err := bt.InstallPackage(context.Background(), &InstallPackageRequest{Packages: []string{"lodash", "vite"}, Dev: true})
	if err != nil {
		t.Fatalf("InstallPackage returned error: %v", err)
	}
	if !installResp.Success || strings.Join(installResp.Installed, ",") != "lodash,vite" {
		t.Fatalf("install response = %#v", installResp)
	}

	globalResp, err := bt.InstallPackage(context.Background(), &InstallPackageRequest{Packages: []string{"tsx"}, Global: true})
	if err != nil {
		t.Fatalf("global InstallPackage returned error: %v", err)
	}
	if !globalResp.Success {
		t.Fatalf("global install response = %#v", globalResp)
	}

	removeResp, err := bt.RemovePackage(context.Background(), &RemovePackageRequest{Packages: []string{"lodash"}})
	if err != nil {
		t.Fatalf("RemovePackage returned error: %v", err)
	}
	if !removeResp.Success || strings.Join(removeResp.Removed, ",") != "lodash" {
		t.Fatalf("remove response = %#v", removeResp)
	}

	log := readLog(t, logPath)
	for _, want := range []string{"add -d lodash vite", "install -g tsx", "remove lodash"} {
		if !strings.Contains(log, want) {
			t.Fatalf("command log missing %q: %q", want, log)
		}
	}
}

func TestBunPackageValidationErrors(t *testing.T) {
	bt := newTestBunTools(t)

	if resp, err := bt.InstallPackage(context.Background(), &InstallPackageRequest{}); err != nil || resp.Success || resp.ErrorMessage == "" {
		t.Fatalf("empty install resp=%#v err=%v", resp, err)
	}
	if resp, err := bt.RemovePackage(context.Background(), &RemovePackageRequest{}); err != nil || resp.Success || resp.ErrorMessage == "" {
		t.Fatalf("empty remove resp=%#v err=%v", resp, err)
	}
	if resp, err := bt.InstallPackage(context.Background(), &InstallPackageRequest{Packages: []string{"lodash"}}); err != nil || resp.Success || !strings.Contains(resp.ErrorMessage, "项目未初始化") {
		t.Fatalf("missing package.json install resp=%#v err=%v", resp, err)
	}
	if resp, err := bt.RemovePackage(context.Background(), &RemovePackageRequest{Packages: []string{"lodash"}}); err != nil || resp.Success || !strings.Contains(resp.ErrorMessage, "项目未初始化") {
		t.Fatalf("missing package.json remove resp=%#v err=%v", resp, err)
	}
}

func TestBunListPackage(t *testing.T) {
	bt := newTestBunTools(t)

	if resp, err := bt.ListPackage(context.Background(), &ListPackageRequest{}); err != nil || resp.Success || !strings.Contains(resp.ErrorMessage, "项目未初始化") {
		t.Fatalf("missing package.json list resp=%#v err=%v", resp, err)
	}

	writePackageJSON(t, bt, `{"dependencies":{"lodash":"^4.17.21"},"devDependencies":{"vite":"^5.0.0"}}`)
	resp, err := bt.ListPackage(context.Background(), &ListPackageRequest{})
	if err != nil {
		t.Fatalf("ListPackage returned error: %v", err)
	}
	if !resp.Success || len(resp.Packages) != 2 {
		t.Fatalf("list response = %#v", resp)
	}
	packages := map[string]string{}
	for _, pkg := range resp.Packages {
		packages[pkg.Name] = pkg.Version
	}
	if packages["lodash"] != "^4.17.21" || packages["vite"] != "^5.0.0 (dev)" {
		t.Fatalf("packages = %#v", packages)
	}

	writePackageJSON(t, bt, `{bad json`)
	if resp, err := bt.ListPackage(context.Background(), &ListPackageRequest{}); err != nil || resp.Success || !strings.Contains(resp.ErrorMessage, "解析 package.json") {
		t.Fatalf("invalid package.json list resp=%#v err=%v", resp, err)
	}
}

func TestBunCleanEnv(t *testing.T) {
	bt := newTestBunTools(t)
	writePackageJSON(t, bt, `{"dependencies":{}}`)
	if err := os.MkdirAll(filepath.Join(bt.envDir, "node_modules"), 0755); err != nil {
		t.Fatalf("create node_modules: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bt.envDir, "bun.lockb"), []byte("lock"), 0644); err != nil {
		t.Fatalf("write bun.lockb: %v", err)
	}

	resp, err := bt.CleanEnv(context.Background(), &CleanEnvRequest{KeepPackageJSON: true})
	if err != nil {
		t.Fatalf("CleanEnv returned error: %v", err)
	}
	if !resp.Success || !strings.Contains(resp.Message, "保留了 package.json") {
		t.Fatalf("clean response = %#v", resp)
	}
	if _, err := os.Stat(filepath.Join(bt.envDir, "package.json")); err != nil {
		t.Fatalf("package.json should be kept: %v", err)
	}
	if _, err := os.Stat(filepath.Join(bt.envDir, "node_modules")); !os.IsNotExist(err) {
		t.Fatalf("node_modules should be removed, stat err=%v", err)
	}

	resp, err = bt.CleanEnv(context.Background(), &CleanEnvRequest{})
	if err != nil {
		t.Fatalf("CleanEnv full returned error: %v", err)
	}
	if !resp.Success {
		t.Fatalf("full clean response = %#v", resp)
	}
	if _, err := os.Stat(filepath.Join(bt.envDir, "package.json")); !os.IsNotExist(err) {
		t.Fatalf("package.json should be removed, stat err=%v", err)
	}
}

func TestBunRunScriptValidation(t *testing.T) {
	bt := newTestBunTools(t)

	if resp, err := bt.RunScript(context.Background(), &RunScriptRequest{}); err != nil || resp.Success || !strings.Contains(resp.ErrorMessage, "必须提供") {
		t.Fatalf("RunScript missing input resp=%#v err=%v", resp, err)
	}
	if resp, err := bt.RunScript(context.Background(), &RunScriptRequest{ScriptPath: "missing.js"}); err != nil || resp.Success || !strings.Contains(resp.ErrorMessage, "脚本文件不存在") {
		t.Fatalf("RunScript missing file resp=%#v err=%v", resp, err)
	}
}

func TestBunGetEnvInfo(t *testing.T) {
	bt := newTestBunTools(t)

	resp, err := bt.GetEnvInfo(context.Background(), &GetEnvInfoRequest{})
	if err != nil {
		t.Fatalf("GetEnvInfo returned error: %v", err)
	}
	if !resp.Success || resp.Initialized || !strings.Contains(resp.ErrorMessage, "项目未初始化") {
		t.Fatalf("missing env info response = %#v", resp)
	}

	writePackageJSON(t, bt, `{"dependencies":{"lodash":"^4.17.21"}}`)
	if err := os.MkdirAll(filepath.Join(bt.envDir, "node_modules"), 0755); err != nil {
		t.Fatalf("create node_modules: %v", err)
	}
	t.Setenv("BUN_TEST_LOG", filepath.Join(t.TempDir(), "bun.log"))

	resp, err = bt.GetEnvInfo(context.Background(), &GetEnvInfoRequest{})
	if err != nil {
		t.Fatalf("GetEnvInfo initialized returned error: %v", err)
	}
	if !resp.Success || !resp.Initialized || resp.PackageCount != 1 || !resp.HasNodeModules || resp.BunVersion != "1.2.3" {
		t.Fatalf("initialized env info response = %#v", resp)
	}
}
