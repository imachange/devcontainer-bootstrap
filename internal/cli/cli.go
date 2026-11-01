// Package cli は、サブコマンドの振り分けを行います。
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/imachange/devcontainer-bootstrap/internal/selector"
	"github.com/imachange/devcontainer-bootstrap/internal/version"
)

const binName = "devcontainer-bootstrap"

// App は、コマンドの実行環境です。
type App struct {
	Stdout, Stderr io.Writer
	Deps           selector.Deps
	Boot           BootDeps
}

// NewApp は、実際の標準入出力と環境を使う App を返します。
func NewApp() *App {
	return &App{Stdout: os.Stdout, Stderr: os.Stderr, Deps: selector.RealDeps(), Boot: RealBootDeps()}
}

// Run は、コマンドを実行して終了コードを返します。引数が無いときは bootstrap を実行します。
func (a *App) Run(args []string) int {
	if len(args) == 0 {
		return a.bootstrap(nil)
	}
	switch args[0] {
	case "bootstrap":
		return a.bootstrap(args[1:])
	case "version":
		// リリースタグだけを 1 行で出力する (bootstrap.sh と select が依存する契約)
		fmt.Fprintln(a.Stdout, version.Get())
		return 0
	case "select":
		return a.selectCmd(args[1:])
	case "help", "-h", "--help":
		a.usage(a.Stdout)
		return 0
	default:
		fmt.Fprintf(a.Stderr, "unknown command: %s\n\n", args[0])
		a.usage(a.Stderr)
		return 2
	}
}

func (a *App) usage(w io.Writer) {
	fmt.Fprint(w, `Usage: devcontainer-bootstrap [command]

Commands:
  bootstrap [--root DIR]  ホスト側の初期化を実行する (省略時の既定)
  version                 リリースタグだけを出力する
  select [--want T]       bin/ のバイナリと PATH のコマンドのどちらを使うかを、bin / path / none の 1 語で出力する
  help                    このヘルプを表示する

Environment (bootstrap):
  DEVBOOTSTRAP_BUILDER_REPO     Dockerfile の取得元リポジトリ (既定: imachange/devcontainer-builder)
  DEVBOOTSTRAP_BUILDER_VERSION  Dockerfile のリリースタグ (未指定なら最新)
  DEVBOOTSTRAP_SKIP_INITIALIZE  1 なら initialize.* を実行しない
`)
}

// selectCmd は、bin/ のバイナリにだけ bootstrap.sh が実行する判定コマンドです。
// 標準出力には、bin / path / none の 1 語だけを出します。理由は標準エラーに出します。
func (a *App) selectCmd(args []string) int {
	fs := flag.NewFlagSet("select", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	want := fs.String("want", "", "固定するバージョン (リリースタグ)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(a.Stderr, "select: unexpected arguments: %v\n", fs.Args())
		return 2
	}
	given := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "want" {
			given = true
		}
	})
	if given && *want == "" {
		fmt.Fprintln(a.Stderr, "select: --want requires a non-empty value")
		return 2
	}

	decision, notes := selector.Run(context.Background(), a.Deps, binName, version.Get(), *want)
	for _, n := range notes {
		fmt.Fprintf(a.Stderr, "select: %s\n", n)
	}
	fmt.Fprintln(a.Stdout, decision)
	return 0
}
