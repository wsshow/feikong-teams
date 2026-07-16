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
	"time"

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

type providerRegistryProvider struct {
	name string
}

func (p providerRegistryProvider) Name() string { return p.name }

func (p providerRegistryProvider) Search(context.Context, string, int, int, string, string) (*SearchResponse, error) {
	return &SearchResponse{}, nil
}

func (p providerRegistryProvider) Download(context.Context, string, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func TestProviderRegistrySelectsProvidersExplicitly(t *testing.T) {
	first := providerRegistryProvider{name: "SkillHub"}
	second := providerRegistryProvider{name: "Mirror"}
	registry := NewProviderRegistry(nil, first, second)

	if got := registry.DefaultProvider(); got == nil || got.Name() != "SkillHub" {
		t.Fatalf("DefaultProvider = %#v, want SkillHub", got)
	}
	if got := registry.ProviderByName("mirror"); got == nil || got.Name() != "Mirror" {
		t.Fatalf("ProviderByName = %#v, want Mirror", got)
	}

	selected, err := registry.ProvidersByNames([]string{"skillhub", "mirror"})
	if err != nil {
		t.Fatalf("ProvidersByNames returned error: %v", err)
	}
	if len(selected) != 2 || selected[0].Name() != "SkillHub" || selected[1].Name() != "Mirror" {
		t.Fatalf("selected providers = %#v", selected)
	}
	if _, err := registry.ProvidersByNames([]string{"missing"}); err == nil || !strings.Contains(err.Error(), "skill provider not found") {
		t.Fatalf("missing provider error = %v, want skill provider not found", err)
	}
}

func TestProviderRegistryReturnsCopies(t *testing.T) {
	registry := NewProviderRegistry(providerRegistryProvider{name: "one"})
	providers := registry.Providers()
	providers[0] = providerRegistryProvider{name: "mutated"}

	if got := registry.DefaultProvider().Name(); got != "one" {
		t.Fatalf("registry provider mutated to %q, want one", got)
	}
	if names := registry.Names(); len(names) != 1 || names[0] != "one" {
		t.Fatalf("Names = %#v, want one", names)
	}
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

func TestSkillFileOperationsRejectSymlinks(t *testing.T) {
	appDir := t.TempDir()
	t.Setenv(env.AppDir, appDir)
	writeSkill(t, appDir, "demo", "---\nname: Demo\n---\n")
	outDir := t.TempDir()
	outside := filepath.Join(outDir, "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(appDir, "skills", "demo", "link.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}

	if _, err := ReadSkillFile("demo", "link.txt"); err == nil {
		t.Fatal("ReadSkillFile followed a symbolic link")
	}
	if err := SaveSkillFile("demo", "link.txt", "changed"); err == nil {
		t.Fatal("SaveSkillFile replaced a symbolic link")
	}
	if err := DeleteSkillFile("demo", "link.txt"); err == nil {
		t.Fatal("DeleteSkillFile accepted a symbolic link")
	}
	data, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "outside" {
		t.Fatalf("outside file = %q", data)
	}
}

func TestReadSkillRootFileRejectsOversizedContent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "large.txt"), []byte("12345"), 0644); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if _, err := readSkillRootFile(root, "large.txt", 4); err == nil {
		t.Fatal("oversized skill file was accepted")
	}
}

func TestCreateSaveAndDeleteLocalSkillFiles(t *testing.T) {
	appDir := t.TempDir()
	t.Setenv(env.AppDir, appDir)

	info, err := CreateLocalSkill(LocalSkillSpec{
		Slug:        "custom_skill",
		Name:        "Custom Skill",
		Description: "Custom description",
	})
	if err != nil {
		t.Fatalf("CreateLocalSkill returned error: %v", err)
	}
	if info.Slug != "custom_skill" || info.Name != "Custom Skill" {
		t.Fatalf("created skill = %#v", info)
	}

	if err := CreateSkillFile("custom_skill", "references", "", true); err != nil {
		t.Fatalf("CreateSkillFile dir returned error: %v", err)
	}
	if err := CreateSkillFile("custom_skill", "references/example.md", "example", false); err != nil {
		t.Fatalf("CreateSkillFile file returned error: %v", err)
	}
	if err := SaveSkillFile("custom_skill", "references/example.md", "updated"); err != nil {
		t.Fatalf("SaveSkillFile returned error: %v", err)
	}
	content, err := ReadSkillFile("custom_skill", "references/example.md")
	if err != nil {
		t.Fatalf("ReadSkillFile returned error: %v", err)
	}
	if content != "updated" {
		t.Fatalf("content = %q, want updated", content)
	}
	if err := DeleteSkillFile("custom_skill", "references/example.md"); err != nil {
		t.Fatalf("DeleteSkillFile returned error: %v", err)
	}
	if _, err := ReadSkillFile("custom_skill", "references/example.md"); err == nil {
		t.Fatal("deleted file should not be readable")
	}
}

func TestSkillMutationsRejectInvalidPaths(t *testing.T) {
	appDir := t.TempDir()
	t.Setenv(env.AppDir, appDir)
	writeSkill(t, appDir, "demo", "---\nname: Demo\n---\n")

	if _, err := CreateLocalSkill(LocalSkillSpec{Slug: "../bad", Name: "Bad"}); err == nil || !strings.Contains(err.Error(), "invalid skill slug") {
		t.Fatalf("CreateLocalSkill traversal error = %v, want invalid skill slug", err)
	}
	if err := SaveSkillFile("demo", "../secret.txt", "secret"); err == nil || !strings.Contains(err.Error(), "invalid path") {
		t.Fatalf("SaveSkillFile traversal error = %v, want invalid path", err)
	}
	if err := CreateSkillFile("demo", "/secret.txt", "secret", false); err == nil || !strings.Contains(err.Error(), "invalid path") {
		t.Fatalf("CreateSkillFile absolute error = %v, want invalid path", err)
	}
	if err := DeleteSkillFile("demo", "SKILL.md"); err == nil || !strings.Contains(err.Error(), "cannot be deleted") {
		t.Fatalf("DeleteSkillFile SKILL.md error = %v, want cannot be deleted", err)
	}
}

func TestListSkillFilesSortsByTypeModTimeSizeAndName(t *testing.T) {
	appDir := t.TempDir()
	t.Setenv(env.AppDir, appDir)
	writeSkill(t, appDir, "demo", "---\nname: Demo\n---\n")

	skillDir := filepath.Join(appDir, "skills", "demo")
	for _, dir := range []string{"old-dir", "new-dir"} {
		if err := os.Mkdir(filepath.Join(skillDir, dir), 0755); err != nil {
			t.Fatalf("create dir %s: %v", dir, err)
		}
	}
	for name, content := range map[string]string{
		"recent-small.txt": "r",
		"old-large.txt":    strings.Repeat("o", 64),
		"same-large.txt":   strings.Repeat("l", 32),
		"same-alpha.txt":   "s",
		"same-beta.txt":    "s",
	} {
		if err := os.WriteFile(filepath.Join(skillDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("write file %s: %v", name, err)
		}
	}

	base := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	modTimes := map[string]time.Time{
		"new-dir":          base.Add(2 * time.Hour),
		"old-dir":          base.Add(time.Hour),
		"recent-small.txt": base.Add(4 * time.Hour),
		"old-large.txt":    base,
		"same-large.txt":   base.Add(3 * time.Hour),
		"same-alpha.txt":   base.Add(3 * time.Hour),
		"same-beta.txt":    base.Add(3 * time.Hour),
		"SKILL.md":         base.Add(-time.Hour),
	}
	for name, modTime := range modTimes {
		path := filepath.Join(skillDir, name)
		if err := os.Chtimes(path, modTime, modTime); err != nil {
			t.Fatalf("change mod time %s: %v", name, err)
		}
	}

	entries, err := ListSkillFiles("demo", "")
	if err != nil {
		t.Fatalf("ListSkillFiles returned error: %v", err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name)
	}
	want := []string{"new-dir", "old-dir", "recent-small.txt", "same-large.txt", "same-alpha.txt", "same-beta.txt", "old-large.txt", "SKILL.md"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("entry order = %#v, want %#v", names, want)
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
	if err := InstallSkillFromProvider(context.Background(), "bad", "", fakeProvider{data: slipData}); err == nil || !strings.Contains(err.Error(), "invalid skill archive path") {
		t.Fatalf("zip slip install error = %v, want illegal path", err)
	}
	if _, err := os.Stat(filepath.Join(appDir, "skills", "evil.txt")); !os.IsNotExist(err) {
		t.Fatalf("evil file exists or stat error: %v", err)
	}
}

func TestInstallSkillRejectsInvalidSlugWithoutTouchingFilesystem(t *testing.T) {
	appDir := t.TempDir()
	t.Setenv(env.AppDir, appDir)
	if err := InstallSkillFromProvider(context.Background(), "../bad", "", fakeProvider{}); err == nil {
		t.Fatal("invalid skill slug was accepted")
	}
	if _, err := os.Stat(filepath.Join(appDir, "bad")); !os.IsNotExist(err) {
		t.Fatalf("invalid install touched filesystem: %v", err)
	}
}

func TestInstallSkillFailurePreservesPreviousVersion(t *testing.T) {
	appDir := t.TempDir()
	t.Setenv(env.AppDir, appDir)
	writeSkill(t, appDir, "demo", "previous")

	badArchive := makeZip(t, map[string]string{"README.md": "missing required file"})
	if err := InstallSkillFromProvider(context.Background(), "demo", "", fakeProvider{data: badArchive}); err == nil {
		t.Fatal("invalid replacement skill was accepted")
	}
	data, err := os.ReadFile(filepath.Join(appDir, "skills", "demo", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "previous" {
		t.Fatalf("previous skill content = %q", data)
	}
}

func TestWriteSkillArchiveRejectsOversizedDownload(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "skill.zip")
	err := writeSkillArchiveLimited(context.Background(), filePath, strings.NewReader("12345"), 4)
	if err == nil {
		t.Fatal("oversized skill archive was accepted")
	}
	if _, statErr := os.Stat(filePath); !os.IsNotExist(statErr) {
		t.Fatalf("rejected archive was not removed: %v", statErr)
	}
}

func TestUnzipSkillRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "skill.zip")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	header := &zip.FileHeader{Name: "link"}
	header.SetMode(os.ModeSymlink | 0777)
	entry, err := writer.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("target")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := unzipSkill(context.Background(), archivePath, filepath.Join(dir, "out")); err == nil {
		t.Fatal("symlink archive entry was accepted")
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
