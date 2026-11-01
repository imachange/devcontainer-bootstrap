package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/imachange/devcontainer-bootstrap/internal/initscript"
	"github.com/imachange/devcontainer-bootstrap/internal/selector"
)

type fakeGH struct {
	latest    string
	verifyErr error
	content   string
	calls     []string
	repoSeen  string
	tagSeen   string
}

func (f *fakeGH) LatestTag(ctx context.Context, repo string) (string, error) {
	f.calls = append(f.calls, "latest")
	f.repoSeen = repo
	return f.latest, nil
}

func (f *fakeGH) Download(ctx context.Context, repo, tag, asset, dest string) error {
	f.calls = append(f.calls, "download")
	f.repoSeen, f.tagSeen = repo, tag
	return os.WriteFile(dest, []byte(f.content), 0o644)
}

func (f *fakeGH) Verify(ctx context.Context, repo, tag, file string) error {
	f.calls = append(f.calls, "verify")
	return f.verifyErr
}

func (f *fakeGH) called(name string) bool {
	for _, c := range f.calls {
		if c == name {
			return true
		}
	}
	return false
}

type bootEnv struct {
	app    *App
	stdout *bytes.Buffer
	stderr *bytes.Buffer
	ran    []initscript.Command
	exit   int
}

func newBootEnv(gh *fakeGH, env map[string]string) *bootEnv {
	be := &bootEnv{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	be.app = &App{
		Stdout: be.stdout,
		Stderr: be.stderr,
		Deps:   selector.Deps{},
		Boot: BootDeps{
			Getenv: func(k string) string { return env[k] },
			GOOS:   "linux",
			LookPath: func(n string) (string, error) {
				if n == "sh" {
					return "/bin/sh", nil
				}
				return "", errors.New("not found")
			},
			GH:         gh,
			GitIgnored: func(string, string) bool { return false },
			RunScript: func(c initscript.Command, _ io.Reader, _, _ io.Writer) (int, error) {
				be.ran = append(be.ran, c)
				return be.exit, nil
			},
		},
	}
	return be
}

// stubBoot は、何もしない BootDeps です (bootstrap 以外のコマンドのテスト用)。
func stubBoot() BootDeps {
	return newBootEnv(&fakeGH{}, nil).app.Boot
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

const buildJSON = "{\n  // build 方式\n  \"name\": \"x\",\n  \"build\": { \"dockerfile\": \"Dockerfile\", },\n}\n"

func TestBootstrapFetchesDockerfileForBuild(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".devcontainer", "devcontainer.json"), buildJSON)
	gh := &fakeGH{latest: "v1.2.3", content: "FROM scratch\n"}
	be := newBootEnv(gh, map[string]string{"DEVBOOTSTRAP_BUILDER_REPO": "me/builder"})

	if code := be.app.Run([]string{"bootstrap", "--root", root}); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, be.stderr)
	}
	got, err := os.ReadFile(filepath.Join(root, ".devcontainer", "Dockerfile"))
	if err != nil || string(got) != "FROM scratch\n" {
		t.Fatalf("Dockerfile = %q, %v", got, err)
	}
	if gh.repoSeen != "me/builder" || gh.tagSeen != "v1.2.3" {
		t.Errorf("repo=%q tag=%q", gh.repoSeen, gh.tagSeen)
	}

	// 2 回目: 同じリリースなので、再取得しない
	gh.calls = nil
	if code := be.app.Run([]string{"bootstrap", "--root", root}); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, be.stderr)
	}
	if gh.called("download") {
		t.Errorf("再取得した: %v", gh.calls)
	}
}

func TestBootstrapUsesDefaultBuilderRepoAndPinnedVersion(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".devcontainer", "devcontainer.json"), buildJSON)
	gh := &fakeGH{content: "A\n"}
	be := newBootEnv(gh, map[string]string{"DEVBOOTSTRAP_BUILDER_VERSION": "v9.9.9"})
	if code := be.app.Run([]string{"bootstrap", "--root", root}); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, be.stderr)
	}
	if gh.repoSeen != defaultBuilderRepo || gh.tagSeen != "v9.9.9" {
		t.Errorf("repo=%q tag=%q", gh.repoSeen, gh.tagSeen)
	}
	if gh.called("latest") {
		t.Error("固定版なら、最新の問い合わせはしないはず")
	}
}

func TestBootstrapSkipsDockerfileWithoutBuildOnly(t *testing.T) {
	for name, src := range map[string]string{
		"image":       `{"image": "ubuntu"}`,
		"image+build": `{"image": "ubuntu", "build": {}}`,
		"なし":          `{"name": "x"}`,
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, filepath.Join(root, ".devcontainer", "devcontainer.json"), src)
			gh := &fakeGH{latest: "v1.0.0", content: "A\n"}
			be := newBootEnv(gh, nil)
			if code := be.app.Run([]string{"bootstrap", "--root", root}); code != 0 {
				t.Fatalf("code=%d stderr=%s", code, be.stderr)
			}
			if len(gh.calls) != 0 {
				t.Errorf("gh を呼ばないはず: %v", gh.calls)
			}
		})
	}
}

func TestBootstrapWithoutDevcontainerJSON(t *testing.T) {
	gh := &fakeGH{}
	be := newBootEnv(gh, nil)
	if code := be.app.Run([]string{"bootstrap", "--root", t.TempDir()}); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, be.stderr)
	}
	if len(gh.calls) != 0 || len(be.ran) != 0 {
		t.Errorf("何も実行しないはず: calls=%v ran=%v", gh.calls, be.ran)
	}
}

func TestBootstrapRunsInitializeScript(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".devcontainer", "devcontainer.json"), `{"image": "x"}`)
	writeFile(t, filepath.Join(root, ".devcontainer", "script", "initialize.sh"), "echo hi\n")
	be := newBootEnv(&fakeGH{}, nil)

	if code := be.app.Run([]string{"bootstrap", "--root", root}); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, be.stderr)
	}
	if len(be.ran) != 1 {
		t.Fatalf("ran = %v", be.ran)
	}
	c := be.ran[0]
	if c.Name != "/bin/sh" || len(c.Args) != 1 || c.Args[0] != ".devcontainer/script/initialize.sh" || c.Dir != root {
		t.Errorf("command = %+v", c)
	}
}

func TestBootstrapPropagatesInitializeExitCode(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".devcontainer", "script", "initialize.sh"), "exit 3\n")
	be := newBootEnv(&fakeGH{}, nil)
	be.exit = 3
	if code := be.app.Run([]string{"bootstrap", "--root", root}); code != 3 {
		t.Errorf("code=%d, want 3", code)
	}
}

func TestBootstrapSkipInitialize(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".devcontainer", "script", "initialize.sh"), "echo hi\n")
	be := newBootEnv(&fakeGH{}, map[string]string{"DEVBOOTSTRAP_SKIP_INITIALIZE": "1"})
	if code := be.app.Run([]string{"bootstrap", "--root", root}); code != 0 {
		t.Fatalf("code=%d", code)
	}
	if len(be.ran) != 0 {
		t.Errorf("実行しないはず: %v", be.ran)
	}
}

func TestBootstrapDockerfileErrorStopsBeforeInitialize(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".devcontainer", "devcontainer.json"), buildJSON)
	writeFile(t, filepath.Join(root, ".devcontainer", "script", "initialize.sh"), "echo hi\n")
	gh := &fakeGH{latest: "v1.0.0", content: "tampered\n", verifyErr: errors.New("mismatch")}
	be := newBootEnv(gh, nil)
	if code := be.app.Run([]string{"bootstrap", "--root", root}); code != 1 {
		t.Errorf("code=%d, want 1", code)
	}
	if len(be.ran) != 0 {
		t.Errorf("initialize を実行しないはず: %v", be.ran)
	}
	if _, err := os.Stat(filepath.Join(root, ".devcontainer", "Dockerfile")); err == nil {
		t.Error("検証に失敗したのに、Dockerfile が置かれた")
	}
}

func TestBootstrapBadArguments(t *testing.T) {
	be := newBootEnv(&fakeGH{}, nil)
	for _, args := range [][]string{{"bootstrap", "--bogus"}, {"bootstrap", "extra"}} {
		if code := be.app.Run(args); code != 2 {
			t.Errorf("%v: code=%d, want 2", args, code)
		}
	}
}

func TestBootstrapRequiresDependencies(t *testing.T) {
	app := &App{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
	if code := app.Run([]string{"bootstrap"}); code != 1 {
		t.Errorf("code=%d, want 1", code)
	}
}
