// Package devcontainer は、devcontainer.json から bootstrap に必要な情報だけを読み取ります。
package devcontainer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/imachange/devcontainer-bootstrap/internal/jsonc"
)

// Spec は、devcontainer.json から読み取った内容です。
type Spec struct {
	// BuildOnly は、build があり、image と dockerComposeFile が無いときに true です。
	BuildOnly bool
	// Dockerfile は、build.dockerfile (既定: Dockerfile)。devcontainer.json のあるディレクトリからの相対パスです。
	Dockerfile string
}

// Parse は、devcontainer.json (JSONC) を読みます。
func Parse(src []byte) (Spec, error) {
	top, err := jsonc.TopLevel(src)
	if err != nil {
		return Spec{}, err
	}
	spec := Spec{Dockerfile: "Dockerfile"}
	build, hasBuild := present(top, "build")
	_, hasImage := present(top, "image")
	_, hasCompose := present(top, "dockerComposeFile")
	spec.BuildOnly = hasBuild && !hasImage && !hasCompose
	if hasBuild {
		var b struct {
			Dockerfile string `json:"dockerfile"`
		}
		if err := json.Unmarshal(build, &b); err != nil {
			return Spec{}, fmt.Errorf("build: %w", err)
		}
		if b.Dockerfile != "" {
			spec.Dockerfile = b.Dockerfile
		}
	}
	return spec, nil
}

// present は、キーがあり、値が null でないときに、その値を返します。
func present(top map[string]json.RawMessage, key string) (json.RawMessage, bool) {
	raw, ok := top[key]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, false
	}
	return raw, true
}

// DockerfilePath は、devcontainer.json のあるディレクトリ dir と、build.dockerfile の値 rel から、
// Dockerfile の絶対パスを返します。dir の外を指す値は拒否します。
func DockerfilePath(dir, rel string) (string, error) {
	if rel == "" || strings.ContainsRune(rel, 0) {
		return "", errors.New("invalid build.dockerfile")
	}
	if strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, `\`) || filepath.IsAbs(rel) || filepath.VolumeName(rel) != "" {
		return "", fmt.Errorf("build.dockerfile must be a relative path: %q", rel)
	}
	p := filepath.Join(dir, filepath.FromSlash(rel))
	r, err := filepath.Rel(dir, p)
	if err != nil || r == "." || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("build.dockerfile points outside of %s: %q", dir, rel)
	}
	return p, nil
}
