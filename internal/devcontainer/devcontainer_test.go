package devcontainer

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name       string
		src        string
		buildOnly  bool
		dockerfile string
	}{
		{"build のみ", `{"build": {"dockerfile": "Dockerfile"}}`, true, "Dockerfile"},
		{"build の dockerfile 省略", `{"build": {"context": ".."}}`, true, "Dockerfile"},
		{"build の dockerfile 指定", `{"build": {"dockerfile": "docker/Dev.Dockerfile"}}`, true, "docker/Dev.Dockerfile"},
		{"image のみ", `{"image": "ubuntu"}`, false, "Dockerfile"},
		{"image と build", `{"image": "ubuntu", "build": {"dockerfile": "Dockerfile"}}`, false, "Dockerfile"},
		{"compose と build", `{"dockerComposeFile": "compose.yml", "build": {}}`, false, "Dockerfile"},
		{"build が null", `{"build": null}`, false, "Dockerfile"},
		{"何も無い", `{"name": "x"}`, false, "Dockerfile"},
		{"image が null", `{"image": null, "build": {}}`, true, "Dockerfile"},
		{"ほかのキーがあっても読める", `{"name": "x", "features": {}, "forwardPorts": [3000], "customizations": {"vscode": {}}, "unknownKey": 1, "build": {"dockerfile": "Dockerfile"}}`, true, "Dockerfile"},
		{"JSONC (コメントと末尾カンマ)", "{\n // c\n \"build\": { \"dockerfile\": \"Dockerfile\", },\n}\n", true, "Dockerfile"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse([]byte(tt.src))
			if err != nil {
				t.Fatal(err)
			}
			if got.BuildOnly != tt.buildOnly || got.Dockerfile != tt.dockerfile {
				t.Errorf("got %+v, want BuildOnly=%v Dockerfile=%q", got, tt.buildOnly, tt.dockerfile)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	for _, src := range []string{`{"build": }`, `{"build": "string"}`, `[]`} {
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("Parse(%q): エラーになるはず", src)
		}
	}
}

func TestDockerfilePath(t *testing.T) {
	dir := t.TempDir()
	ok := map[string]string{
		"Dockerfile":        filepath.Join(dir, "Dockerfile"),
		"docker/Dockerfile": filepath.Join(dir, "docker", "Dockerfile"),
		"./Dockerfile":      filepath.Join(dir, "Dockerfile"),
		"a/../Dockerfile":   filepath.Join(dir, "Dockerfile"),
	}
	for rel, want := range ok {
		got, err := DockerfilePath(dir, rel)
		if err != nil || got != want {
			t.Errorf("DockerfilePath(%q) = %q, %v; want %q", rel, got, err, want)
		}
	}
	for _, rel := range []string{"", "../Dockerfile", "a/../../Dockerfile", "/etc/Dockerfile", `\Dockerfile`, ".", "a/.."} {
		if got, err := DockerfilePath(dir, rel); err == nil {
			t.Errorf("DockerfilePath(%q) = %q; エラーになるはず", rel, got)
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoad(t *testing.T) {
	const build = `{"build": {"dockerfile": "Dockerfile"}}`

	tests := []struct {
		name      string
		files     map[string]string // root からの相対パス → 内容
		wantPath  string            // root からの相対パス
		buildOnly bool
	}{
		{"1. .devcontainer/devcontainer.json", map[string]string{".devcontainer/devcontainer.json": build}, ".devcontainer/devcontainer.json", true},
		{"2. .devcontainer/<DIR>/devcontainer.json", map[string]string{".devcontainer/dev/devcontainer.json": build}, ".devcontainer/dev/devcontainer.json", true},
		{"3. .devcontainer.json", map[string]string{".devcontainer.json": build}, ".devcontainer.json", true},
		{"image 方式", map[string]string{".devcontainer.json": `{"image": "ubuntu"}`}, ".devcontainer.json", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for rel, content := range tt.files {
				writeFile(t, filepath.Join(root, filepath.FromSlash(rel)), content)
			}
			got, err := Load(root)
			if err != nil {
				t.Fatal(err)
			}
			if want := filepath.Join(root, filepath.FromSlash(tt.wantPath)); got.Path != want {
				t.Errorf("Path = %q, want %q", got.Path, want)
			}
			if got.BuildOnly != tt.buildOnly {
				t.Errorf("BuildOnly = %v, want %v", got.BuildOnly, tt.buildOnly)
			}
		})
	}
}

func TestLoadNotFound(t *testing.T) {
	_, err := Load(t.TempDir())
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestLoadBroken(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".devcontainer", "devcontainer.json"), `{"build": }`)
	_, err := Load(root)
	if err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want 構文エラー", err)
	}
	if !strings.Contains(err.Error(), "devcontainer.json") {
		t.Errorf("エラーに、ファイルのパスが入っていない: %v", err)
	}
}

func TestLoadDiagnostics(t *testing.T) {
	root := t.TempDir()
	// name は、スキーマにあるが bootstrap は使わない (出さない)。myCustomKey は、スキーマに無い (警告)。
	writeFile(t, filepath.Join(root, ".devcontainer", "devcontainer.json"), `{"name": "x", "myCustomKey": 1, "build": {}}`)
	writeFile(t, filepath.Join(root, ".devcontainer.json"), `{"image": "ubuntu"}`)

	got, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Warnings) != 1 || !strings.HasPrefix(got.Warnings[0], "myCustomKey: ") {
		t.Errorf("Warnings = %q", got.Warnings)
	}
	if len(got.Notices) != 1 || !strings.Contains(got.Notices[0], ".devcontainer.json") {
		t.Errorf("Notices = %q", got.Notices)
	}
	if !got.BuildOnly || filepath.Base(filepath.Dir(got.Path)) != ".devcontainer" {
		t.Errorf("優先順位 1 のファイルを使うはず: %+v", got)
	}
}
