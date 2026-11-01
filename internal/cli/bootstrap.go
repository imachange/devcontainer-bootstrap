package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/imachange/devcontainer-bootstrap/internal/devcontainer"
	"github.com/imachange/devcontainer-bootstrap/internal/dockerfile"
	"github.com/imachange/devcontainer-bootstrap/internal/initscript"
	"github.com/imachange/devcontainer-bootstrap/internal/version"
)

// defaultBuilderRepo は、公式の Dockerfile を配布するリポジトリの既定値です。
const defaultBuilderRepo = "imachange/devcontainer-builder"

// BootDeps は、bootstrap が使う外部とのやり取りです。テストでは差し替えます。
type BootDeps struct {
	Getenv     func(key string) string
	GOOS       string
	LookPath   func(file string) (string, error)
	GH         dockerfile.GH
	GitIgnored func(root, path string) bool
	RunScript  initscript.Runner
	Stdin      io.Reader
}

// RealBootDeps は、実際の環境に対する BootDeps を返します。
func RealBootDeps() BootDeps {
	return BootDeps{
		Getenv:   os.Getenv,
		GOOS:     runtime.GOOS,
		LookPath: exec.LookPath,
		GH:       dockerfile.RealGH{},
		GitIgnored: func(root, path string) bool {
			return exec.Command("git", "-C", root, "check-ignore", "-q", "--", path).Run() == nil
		},
		RunScript: initscript.ExecRunner,
		Stdin:     os.Stdin,
	}
}

func (a *App) logf(format string, args ...any) {
	fmt.Fprintf(a.Stderr, "[bootstrap] "+format+"\n", args...)
}

// bootstrap は、ホスト側の初期化の本体です。次の 2 つを、この順に実行します。
//  1. devcontainer.json に build があれば、公式の Dockerfile を取得する
//  2. .devcontainer/script/initialize.* があれば、1 本だけ実行する
//
// 2 つは独立しています。1 が何もしなくても、失敗しても、2 は必ず実行します。
// 終了コードは、2 が失敗したときはその終了コード、2 が成功して 1 だけが失敗したときは 1 です。
func (a *App) bootstrap(args []string) int {
	if a.Boot.Getenv == nil || a.Boot.GH == nil || a.Boot.RunScript == nil || a.Boot.LookPath == nil || a.Boot.GitIgnored == nil {
		a.logf("Error: bootstrap dependencies are not configured")
		return 1
	}

	flags := flag.NewFlagSet("bootstrap", flag.ContinueOnError)
	flags.SetOutput(a.Stderr)
	rootFlag := flags.String("root", "", "リポジトリのルート (省略時は DEVBOOTSTRAP_ROOT、なければ、カレントディレクトリから上に向かって .devcontainer を探す)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() > 0 {
		a.logf("Error: unexpected arguments: %v", flags.Args())
		return 2
	}
	root, err := a.resolveRoot(*rootFlag)
	if err != nil {
		a.logf("Error: %v", err)
		return 1
	}
	a.logf("%s %s (root=%s)", binName, version.Get(), root)
	if info, err := os.Stat(filepath.Join(root, ".devcontainer")); err != nil || !info.IsDir() {
		a.logf("Warning: %s not found; nothing to do (run from the repository root, or use --root / DEVBOOTSTRAP_ROOT)", filepath.Join(root, ".devcontainer"))
		return 0
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	dockerfileErr := a.syncDockerfile(ctx, root)
	if dockerfileErr != nil {
		a.logf("Error: %v", dockerfileErr)
	}
	if code := a.runInitialize(root); code != 0 {
		return code
	}
	if dockerfileErr != nil {
		return 1
	}
	return 0
}

// syncDockerfile は、build 方式のときだけ、公式の Dockerfile を取得します。
func (a *App) syncDockerfile(ctx context.Context, root string) error {
	dcDir := filepath.Join(root, ".devcontainer")
	src, err := os.ReadFile(filepath.Join(dcDir, "devcontainer.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	spec, err := devcontainer.Parse(src)
	if err != nil {
		return fmt.Errorf("devcontainer.json: %w", err)
	}
	if !spec.BuildOnly {
		return nil
	}
	path, err := devcontainer.DockerfilePath(dcDir, spec.Dockerfile)
	if err != nil {
		return err
	}

	repo := a.Boot.Getenv("DEVBOOTSTRAP_BUILDER_REPO")
	if repo == "" {
		repo = defaultBuilderRepo
	}
	out, err := dockerfile.Sync(ctx, a.Boot.GH, dockerfile.Options{
		Repo:      repo,
		Tag:       a.Boot.Getenv("DEVBOOTSTRAP_BUILDER_VERSION"),
		Path:      path,
		IsIgnored: func(p string) bool { return a.Boot.GitIgnored(root, p) },
	})
	if err != nil {
		return err
	}
	rel, relErr := filepath.Rel(root, path)
	if relErr != nil {
		rel = path
	}
	switch out.Result {
	case dockerfile.Written:
		a.logf("fetched %s from %s@%s", filepath.ToSlash(rel), out.Repo, out.Tag)
	case dockerfile.SkippedModified, dockerfile.Kept:
		a.logf("Warning: %s", out.Message)
	}
	return nil
}

// runInitialize は、initialize.* を 1 本だけ実行して、その終了コードを返します。
func (a *App) runInitialize(root string) int {
	if a.Boot.Getenv("DEVBOOTSTRAP_SKIP_INITIALIZE") == "1" {
		return 0
	}
	plan, err := initscript.Find(root, initscript.Deps{GOOS: a.Boot.GOOS, LookPath: a.Boot.LookPath})
	if err != nil {
		a.logf("Error: %v", err)
		return 1
	}
	if plan == nil {
		return 0
	}
	for _, s := range plan.Skipped {
		a.logf("Warning: skipped %s", s)
	}
	a.logf("running %s", plan.Script)
	code, err := a.Boot.RunScript(plan.Cmd, a.Boot.Stdin, a.Stdout, a.Stderr)
	if err != nil {
		a.logf("Error: cannot run %s: %v", plan.Script, err)
		return 1
	}
	if code != 0 {
		a.logf("Error: %s exited with code %d", plan.Script, code)
		if code < 0 {
			return 1
		}
		return code
	}
	return 0
}

// resolveRoot は、リポジトリのルートを決めます。優先順位は次のとおりです。
//  1. --root
//  2. 環境変数 DEVBOOTSTRAP_ROOT (bootstrap.sh が、自分の位置から決めて渡す)
//  3. カレントディレクトリから上に向かって、最初に .devcontainer または .git があるディレクトリ
//  4. カレントディレクトリ
func (a *App) resolveRoot(flagValue string) (string, error) {
	if flagValue != "" {
		return filepath.Abs(flagValue)
	}
	if v := a.Boot.Getenv("DEVBOOTSTRAP_ROOT"); v != "" {
		return filepath.Abs(v)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir := cwd; ; {
		if exists(filepath.Join(dir, ".devcontainer")) || exists(filepath.Join(dir, ".git")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return cwd, nil
		}
		dir = parent
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}