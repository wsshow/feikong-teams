package skill

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fkteams/internal/runtime/env"
)

type fakeProvider struct {
	data []byte
	err  error
}

func (p fakeProvider) Name() string { return "fake" }

func (p fakeProvider) Search(context.Context, string, int, int, string, string) (*SearchResponse, error) {
	return &SearchResponse{}, nil
}

func (p fakeProvider) Download(context.Context, string, string) (io.ReadCloser, error) {
	if p.err != nil {
		return nil, p.err
	}
	return io.NopCloser(bytes.NewReader(p.data)), nil
}

func TestListReadAndRemoveLocalSkills(t *testing.T) {
	appDir := t.TempDir()
	t.Setenv(env.AppDir, appDir)

	writeSkill(t, appDir, "demo", `---
name: Demo Skill
description: |
  Line one
  line two
---
# Demo
`)
	writeSkill(t, appDir, "fallback", `---
description: No explicit name
---
# Fallback
`)
	if err := os.WriteFile(filepath.Join(appDir, "skills", "demo", "notes.txt"), []byte("notes"), 0644); err != nil {
		t.Fatalf("write notes: %v", err)
	}

	skills, err := ListLocalSkills()
	if err != nil {
		t.Fatalf("ListLocalSkills returned error: %v", err)
	}
	if len(skills) != 2 {
		t.Fatalf("skills = %#v, want two skills", skills)
	}
	bySlug := map[string]LocalSkillInfo{}
	for _, s := range skills {
		bySlug[s.Slug] = s
	}
	if bySlug["demo"].Name != "Demo Skill" || bySlug["demo"].Description != "Line one line two" {
		t.Fatalf("demo info = %#v", bySlug["demo"])
	}
	if bySlug["fallback"].Name != "fallback" {
		t.Fatalf("fallback info = %#v, want slug fallback as name", bySlug["fallback"])
	}

	entries, err := ListSkillFiles("demo", "")
	if err != nil {
		t.Fatalf("ListSkillFiles returned error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %#v, want SKILL.md and notes.txt", entries)
	}

	content, err := ReadSkillFile("demo", "notes.txt")
	if err != nil {
		t.Fatalf("ReadSkillFile returned error: %v", err)
	}
	if content != "notes" {
		t.Fatalf("content = %q, want notes", content)
	}

	if err := RemoveLocalSkill("demo"); err != nil {
		t.Fatalf("RemoveLocalSkill returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(appDir, "skills", "demo")); !os.IsNotExist(err) {
		t.Fatalf("skill dir still exists or stat error: %v", err)
	}
}

func TestSkillFilePathTraversalRejected(t *testing.T) {
	appDir := t.TempDir()
	t.Setenv(env.AppDir, appDir)
	writeSkill(t, appDir, "demo", "---\nname: Demo\n---\n")

	if _, err := ListSkillFiles("demo", "../"); err == nil || !strings.Contains(err.Error(), "invalid path") {
		t.Fatalf("ListSkillFiles traversal error = %v, want invalid path", err)
	}
	if _, err := ReadSkillFile("demo", "../secret.txt"); err == nil || !strings.Contains(err.Error(), "invalid path") {
		t.Fatalf("ReadSkillFile traversal error = %v, want invalid path", err)
	}
}

func TestInstallSkillFromProviderExtractsZipAndRejectsZipSlip(t *testing.T) {
	appDir := t.TempDir()
	t.Setenv(env.AppDir, appDir)

	zipData := makeZip(t, map[string]string{
		"SKILL.md":        "---\nname: Demo\n---\n# Demo\n",
		"assets/info.txt": "asset",
	})
	if err := InstallSkillFromProvider(context.Background(), "demo", "1.0.0", fakeProvider{data: zipData}); err != nil {
		t.Fatalf("InstallSkillFromProvider returned error: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(appDir, "skills", "demo", "assets", "info.txt"))
	if err != nil {
		t.Fatalf("read installed asset: %v", err)
	}
	if string(got) != "asset" {
		t.Fatalf("installed asset = %q, want asset", got)
	}

	slipData := makeZip(t, map[string]string{"../evil.txt": "evil"})
	if err := InstallSkillFromProvider(context.Background(), "bad", "", fakeProvider{data: slipData}); err == nil || !strings.Contains(err.Error(), "非法路径") {
		t.Fatalf("zip slip install error = %v, want illegal path", err)
	}
	if _, err := os.Stat(filepath.Join(appDir, "skills", "evil.txt")); !os.IsNotExist(err) {
		t.Fatalf("evil file exists or stat error: %v", err)
	}
}

func writeSkill(t *testing.T, appDir, slug, content string) {
	t.Helper()
	dir := filepath.Join(appDir, "skills", slug)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("create skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
}

func makeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}
