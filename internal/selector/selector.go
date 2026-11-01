// Package selector は、`select` コマンドの判定 (bin/ と PATH のどちらを使うか) を行う。
// 実際の exec は bootstrap.sh が行い、ここは判定だけを返す。
package selector

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/imachange/container-bootstrap/internal/semver"
)

// Decision は、`select` が標準出力に出す。
// この 3 語は互換性のために固定。
type Decision string

const (
	Bin  Decision = "bin"  // bin/ のバイナリを使う
	Path Decision = "path" // PATH 上のコマンドを使う
	None Decision = "none" // どちらも使えない (固定版が、どちらとも一致しない)
)

// Candidate は、PATH 上のコマンドの状態。
type Candidate struct {
	Version string
	OK      bool // false: PATH に無い、bin 自身、または version を実行できなかった
}

// Decide は、判定表に従って結果と理由を返す。副作用はない。
//
//    want あり: 自分が一致 → bin / PATH が一致 → path / どちらも不一致 → none
//    want なし: PATH が無い → bin / 両方 semver で PATH が新しい → path / それ以外 → bin
//
// want の比較は文字列の完全一致。semver の比較は、want が無いときの新しさの判定だけに使用。
func Decide(want, self string, path Candidate) (Decision, string) {
	if want != "" {
		switch {
		case self == want:
			return Bin, "bin のバージョン (" + self + ") が指定と一致"
		case path.OK && path.Version == want:
			return Path, "PATH のバージョン (" + path.Version + ") が指定と一致"
		default:
			return None, "指定 (" + want + ") と一致する bin (" + self + ") も PATH も無い"
		}
	}
	if !path.OK {
		return Bin, "PATH に使えるコマンドが無い"
	}
	pv, pOK := semver.Parse(path.Version)
	sv, sOK := semver.Parse(self)
	if !pOK || !sOK {
		return Bin, "バージョンを比較できない (bin=" + self + ", PATH=" + path.Version + ")"
	}
	if semver.Compare(pv, sv) > 0 {
		return Path, "PATH (" + path.Version + ") の方が bin (" + self + ") より新しい"
	}
	return Bin, "bin (" + self + ") が PATH (" + path.Version + ") 以上"
}

// Deps は、外部とのやり取りです。テストでは差し替える。
type Deps struct {
	Executable func() (string, error)
	LookPath   func(file string) (string, error)
	SameFile   func(a, b string) (bool, error)
	RunVersion func(ctx context.Context, exe string) (string, error)
}

// RealDeps は、実際の環境に対する Deps を返す。
func RealDeps() Deps {
	return Deps{
		Executable: func() (string, error) {
			p, err := os.Executable()
			if err != nil {
				return "", err
			}
			return filepath.EvalSymlinks(p)
		},
		LookPath: exec.LookPath,
		SameFile: func(a, b string) (bool, error) {
			ai, err := os.Stat(a)
			if err != nil {
				return false, err
			}
			bi, err := os.Stat(b)
			if err != nil {
				return false, err
			}
			return os.SameFile(ai, bi), nil
		},
		RunVersion: func(ctx context.Context, exe string) (string, error) {
			ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			out, err := exec.CommandContext(ctx, exe, "version").Output()
			if err != nil {
				return "", err
			}
			line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
			line = strings.TrimSpace(line)
			if line == "" {
				return "", errors.New("version の出力が空")
			}
			return line, nil
		},
	}
}

// Probe は、PATH 上の name を調べる。
// bin 自身と同じファイルは除外する。
// 2 つ目の戻り値は、標準エラーに出す説明。
func Probe(ctx context.Context, d Deps, name string) (Candidate, string) {
	p, err := d.LookPath(name)
	if err != nil {
		return Candidate{}, "PATH に " + name + " が無い"
	}
	if self, err := d.Executable(); err == nil {
		if same, err := d.SameFile(self, p); err == nil && same {
			return Candidate{}, "PATH 上の " + name + " (" + p + ") は bin 自身"
		}
	}
	v, err := d.RunVersion(ctx, p)
	if err != nil {
		return Candidate{}, fmt.Sprintf("%s version を実行できない: %v", p, err)
	}
	return Candidate{Version: v, OK: true}, "PATH 上の " + name + " (" + p + ") は " + v
}

// Run は、PATH を調べて判定する。
// 戻り値の 2 つ目は、標準エラーに出す説明。
func Run(ctx context.Context, d Deps, name, self, want string) (Decision, []string) {
	path, note := Probe(ctx, d, name)
	decision, reason := Decide(want, self, path)
	return decision, []string{note, reason}
}
