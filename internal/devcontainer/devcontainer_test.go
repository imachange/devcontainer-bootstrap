package devcontainer

import (
	"path/filepath"
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
