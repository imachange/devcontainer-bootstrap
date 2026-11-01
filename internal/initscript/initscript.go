// Package initscript は、.devcontainer/script/initialize.* を探して実行します。
package initscript

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Order は、探す拡張子の優先順位です。
// 存在して、かつ実行できるもののうち、最初の 1 本だけを実行します。
var Order = []string{"ps1", "bat", "zsh", "bash", "sh"}

// scriptDir は、スクリプトを探す、ルートからの相対ディレクトリです (スラッシュ区切り)。
const scriptDir = ".devcontainer/script"

// Command は、実行するコマンドです。
type Command struct {
	Name string
	Args []string
	Dir  string
}

// Plan は、実行するスクリプトと、そのコマンドです。
type Plan struct {
	Script  string   // ルートからの相対パス (スラッシュ区切り)
	Cmd     Command  // 作業ディレクトリは、ルート
	Skipped []string // 見つかったが、実行できなかったものと、その理由
}

// Deps は、外部とのやり取りです。テストでは差し替えます。
type Deps struct {
	GOOS     string
	LookPath func(file string) (string, error)
}

// Runner は、コマンドを実行して、終了コードを返します。
type Runner func(c Command, stdin io.Reader, stdout, stderr io.Writer) (int, error)

// Find は、実行するスクリプトを決めます。
// スクリプトが 1 つも無ければ (nil, nil)、あるのに 1 つも実行できなければエラーを返します。
func Find(root string, d Deps) (*Plan, error) {
	dir := filepath.Join(root, filepath.FromSlash(scriptDir))
	var skipped []string
	for _, ext := range Order {
		name := "initialize." + ext
		exists, problem := check(dir, filepath.Join(dir, name))
		if !exists {
			continue
		}
		if problem != "" {
			skipped = append(skipped, name+": "+problem)
			continue
		}
		rel := scriptDir + "/" + name
		cmdName, args, why := interpreter(ext, rel, d)
		if cmdName == "" {
			skipped = append(skipped, name+": "+why)
			continue
		}
		return &Plan{
			Script:  rel,
			Cmd:     Command{Name: cmdName, Args: args, Dir: root},
			Skipped: skipped,
		}, nil
	}
	if len(skipped) > 0 {
		return nil, fmt.Errorf("no runnable initialize.* script: %s", strings.Join(skipped, "; "))
	}
	return nil, nil
}

// check は、path が通常のファイルとして存在するかを調べます。
// シンボリックリンクは、実体が dir の中にあるときだけ許します。
func check(dir, path string) (exists bool, problem string) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, ""
		}
		return true, err.Error()
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := filepath.EvalSymlinks(path)
		if err != nil {
			return true, "cannot resolve symlink: " + err.Error()
		}
		realDir, err := filepath.EvalSymlinks(dir)
		if err != nil {
			return true, err.Error()
		}
		rel, err := filepath.Rel(realDir, target)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true, "symlink points outside of the script directory"
		}
		info, err = os.Stat(target)
		if err != nil {
			return true, err.Error()
		}
	}
	if !info.Mode().IsRegular() {
		return true, "not a regular file"
	}
	return true, ""
}

// interpreter は、拡張子に対応する実行コマンドを返します。使えないときは、名前が空で、理由を返します。
// パスは、作業ディレクトリ (ルート) からの相対パスで渡します (空白を含むパスの引用符の問題を避けるため)。
func interpreter(ext, rel string, d Deps) (string, []string, string) {
	windows := d.GOOS == "windows"
	switch ext {
	case "ps1":
		for _, n := range []string{"pwsh", "powershell"} {
			if n == "powershell" && !windows {
				continue
			}
			if p, err := d.LookPath(n); err == nil {
				args := []string{"-NoProfile"}
				if windows {
					args = append(args, "-ExecutionPolicy", "Bypass")
				}
				return p, append(args, "-File", rel), ""
			}
		}
		return "", nil, "pwsh (or powershell on Windows) not found"
	case "bat":
		if !windows {
			return "", nil, "bat can only run on Windows"
		}
		p, err := d.LookPath("cmd.exe")
		if err != nil {
			return "", nil, "cmd.exe not found"
		}
		return p, []string{"/d", "/c", strings.ReplaceAll(rel, "/", `\`)}, ""
	default: // zsh, bash, sh
		p, err := d.LookPath(ext)
		if err != nil {
			return "", nil, ext + " not found"
		}
		if windows && isWSLLauncher(p) {
			return "", nil, p + " is the WSL launcher"
		}
		return p, []string{rel}, ""
	}
}

// isWSLLauncher は、Windows の System32 にある bash.exe (WSL のランチャー) かを返します。
func isWSLLauncher(p string) bool {
	return strings.Contains(strings.ToLower(strings.ReplaceAll(p, `\`, "/")), "/windows/system32/")
}

// ExecRunner は、実際にコマンドを実行します。
func ExecRunner(c Command, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	cmd := exec.Command(c.Name, c.Args...)
	cmd.Dir = c.Dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	err := cmd.Run()
	if err == nil {
		return 0, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), nil
	}
	return 0, err
}
