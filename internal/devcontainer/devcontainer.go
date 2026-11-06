// Package devcontainer は、devcontainer.json から bootstrap に必要な情報だけを読み取ります。
package devcontainer

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	parser "github.com/imachange/devcontainer-parser"
)

// Spec は、devcontainer.json から読み取った内容です。
type Spec struct {
	// BuildOnly は、build があり、image と dockerComposeFile が無いときに true です。
	BuildOnly bool
	// Dockerfile は、build.dockerfile (既定: Dockerfile)。devcontainer.json のあるディレクトリからの相対パスです。
	Dockerfile string
	// Path は、読み込んだ devcontainer.json のパスです (Parse では空)。
	Path string
	// Warnings は、devcontainer-parser の警告です (スキーマに無いキーなど)。
	Warnings []string
	// Notices は、探索に関する情報です (複数の候補があるときに、どれを使ったか、など)。
	Notices []string
}

// ErrNotFound は、devcontainer.json が見つからないことを表します (errors.Is で判定します)。
var ErrNotFound = parser.ErrNotFound

// supportedKeys は、bootstrap が devcontainer.json から使うトップレベルのキーです。
var supportedKeys = []string{"build", "image", "dockerComposeFile"}

// Load は、root から devcontainer.json を、devcontainer-parser の優先順位で探して読みます。
//  1. .devcontainer/devcontainer.json
//  2. .devcontainer/<DIR>/devcontainer.json
//  3. .devcontainer.json
//
// 見つからないときは、errors.Is(err, ErrNotFound) が true になるエラーを返します。
func Load(root string) (Spec, error) {
	res, err := parser.LoadFromDir(root, parser.WithSupportedKeys(supportedKeys...))
	if err != nil {
		return Spec{}, err
	}
	spec := fromResult(res)
	spec.Path = res.FilePath
	return spec, nil
}

// Parse は、devcontainer.json (JSONC) を、devcontainer-parser で読みます。
func Parse(src []byte) (Spec, error) {
	res, err := parser.Parse(src, parser.WithSupportedKeys(supportedKeys...))
	if err != nil {
		return Spec{}, err
	}
	return fromResult(res), nil
}

func fromResult(res *parser.Result) Spec {
	spec := Spec{Dockerfile: "Dockerfile"}
	_, hasBuild := present(res.UsedFields, "build")
	_, hasImage := present(res.UsedFields, "image")
	_, hasCompose := present(res.UsedFields, "dockerComposeFile")
	spec.BuildOnly = hasBuild && !hasImage && !hasCompose
	if hasBuild && res.Config.Build != nil && res.Config.Build.Dockerfile != "" {
		spec.Dockerfile = res.Config.Build.Dockerfile
	}

	// bootstrap が使わないキーの「無視した」(INFO) は、毎回多くなるので、出さない。
	// 出すのは、警告と、キーに紐づかない情報 (複数の候補がある、など) だけ。
	for _, d := range res.Diagnostics {
		switch {
		case d.Level == parser.LevelWarning && d.Key != "":
			spec.Warnings = append(spec.Warnings, d.Key+": "+d.Message)
		case d.Level == parser.LevelWarning:
			spec.Warnings = append(spec.Warnings, d.Message)
		case d.Level == parser.LevelInfo && d.Key == "":
			spec.Notices = append(spec.Notices, d.Message)
		}
	}
	return spec
}

// present は、キーがあり、値が null でないときに、その値を返します。
func present(fields map[string][]byte, key string) ([]byte, bool) {
	raw, ok := fields[key]
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
