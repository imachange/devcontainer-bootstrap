// Package dockerfile は、公式の Dockerfile を、builder リポジトリのリリース資産として取得します。
// 取得したファイルは、gh release verify-asset で検証してから置きます。
package dockerfile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// AssetName は、builder のリリースに添付する Dockerfile のアセット名です。
const AssetName = "Dockerfile"

// GH は、gh コマンドとのやり取りです。テストでは差し替えます。
type GH interface {
	LatestTag(ctx context.Context, repo string) (string, error)
	Download(ctx context.Context, repo, tag, asset, dest string) error
	Verify(ctx context.Context, repo, tag, file string) error
}

// Options は、Sync の設定です。
type Options struct {
	Repo      string                 // owner/name
	Tag       string                 // 空なら、最新のリリース
	Path      string                 // 書き込み先 (絶対パス)
	IsIgnored func(path string) bool // git の ignore 対象かを返す (nil なら、常に false)
}

// Result は、Sync の結果の種類です。
type Result int

const (
	Written          Result = iota // 取得して書いた
	UpToDate                       // 取得済みで、同じリリース
	SkippedUnmanaged               // 既存のファイルが、取得したものではないので、触らない
	SkippedModified                // 取得したファイルが、編集されているので、触らない
	Kept                           // 取得できなかったが、既存のファイルを使う
)

// Outcome は、Sync の結果です。Message は、利用者に見せる説明です (空のこともあります)。
type Outcome struct {
	Result  Result
	Repo    string
	Tag     string
	Message string
}

// lock は、取得したファイルの隣に置く記録 (<Dockerfile>.bootstrap) です。
type lock struct {
	Repo   string `json:"repo"`
	Tag    string `json:"tag"`
	SHA256 string `json:"sha256"`
}

// Sync は、必要なら Dockerfile を取得して置きます。
//
// 上書きしてよいのは、次のどれかのときだけです。
//   - ファイルが無い
//   - 記録 (<Dockerfile>.bootstrap) があり、ファイルが記録のハッシュと一致する (取得したまま)
//   - 記録は無いが、git の ignore 対象である (旧 bootstrap.sh が取得したもの)
//
// 取得に失敗しても、既存のファイルがあれば、それを使って続行します (Kept)。
// 検証に失敗したときは、必ずエラーにします。
func Sync(ctx context.Context, gh GH, o Options) (Outcome, error) {
	if !ValidRepo(o.Repo) {
		return Outcome{}, fmt.Errorf("invalid builder repo: %q", o.Repo)
	}
	if o.Tag != "" && !ValidTag(o.Tag) {
		return Outcome{}, fmt.Errorf("invalid builder version: %q", o.Tag)
	}

	cur, err := os.ReadFile(o.Path)
	exists := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Outcome{}, err
	}

	lockPath := o.Path + ".bootstrap"
	lk, hasLock := readLock(lockPath)

	if exists {
		switch {
		case hasLock && sum(cur) != lk.SHA256:
			return Outcome{Result: SkippedModified, Message: fmt.Sprintf("%s was modified; not updating it", o.Path)}, nil
		case hasLock:
			// 取得したままのファイル
		case o.IsIgnored != nil && o.IsIgnored(o.Path):
			// 旧仕様: git の ignore 対象なら、取得したものとして扱う
		default:
			return Outcome{Result: SkippedUnmanaged}, nil
		}
	}

	tag := o.Tag
	if tag == "" {
		t, err := gh.LatestTag(ctx, o.Repo)
		if err == nil && !ValidTag(t) {
			err = fmt.Errorf("invalid release tag: %q", t)
		}
		if err != nil {
			if exists {
				return Outcome{Result: Kept, Message: fmt.Sprintf("cannot resolve the latest release of %s; using the existing Dockerfile: %v", o.Repo, err)}, nil
			}
			return Outcome{}, fmt.Errorf("cannot resolve the latest release of %s: %w", o.Repo, err)
		}
		tag = t
	}

	if exists && hasLock && lk.Repo == o.Repo && lk.Tag == tag {
		return Outcome{Result: UpToDate, Repo: o.Repo, Tag: tag}, nil
	}

	if err := os.MkdirAll(filepath.Dir(o.Path), 0o755); err != nil {
		return Outcome{}, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(o.Path), ".dockerfile-*")
	if err != nil {
		return Outcome{}, err
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	if err := gh.Download(ctx, o.Repo, tag, AssetName, tmpPath); err != nil {
		if exists {
			return Outcome{Result: Kept, Repo: o.Repo, Tag: tag, Message: fmt.Sprintf("cannot download %s@%s; using the existing Dockerfile: %v", o.Repo, tag, err)}, nil
		}
		return Outcome{}, fmt.Errorf("cannot download %s@%s: %w", o.Repo, tag, err)
	}
	if err := gh.Verify(ctx, o.Repo, tag, tmpPath); err != nil {
		return Outcome{}, fmt.Errorf("verification failed for %s@%s (the downloaded file was discarded): %w", o.Repo, tag, err)
	}
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return Outcome{}, err
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		return Outcome{}, err
	}
	if err := os.Rename(tmpPath, o.Path); err != nil {
		return Outcome{}, err
	}
	if err := writeLock(lockPath, lock{Repo: o.Repo, Tag: tag, SHA256: sum(data)}); err != nil {
		return Outcome{}, fmt.Errorf("wrote %s but cannot write %s: %w", o.Path, lockPath, err)
	}
	return Outcome{Result: Written, Repo: o.Repo, Tag: tag}, nil
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func readLock(path string) (lock, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return lock{}, false
	}
	var l lock
	if json.Unmarshal(b, &l) != nil || l.SHA256 == "" {
		return lock{}, false
	}
	return l, true
}

func writeLock(path string, l lock) error {
	b, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// ValidRepo は、owner/name の形式で、使える文字だけかを返します。
func ValidRepo(s string) bool {
	owner, name, ok := strings.Cut(s, "/")
	return ok && safeToken(owner) && safeToken(name)
}

// ValidTag は、パスや引数に使っても安全なタグかを返します。
func ValidTag(s string) bool { return safeToken(s) }

func safeToken(s string) bool {
	if s == "" || strings.HasPrefix(s, "-") || strings.Contains(s, "..") {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		ok := c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' ||
			c == '.' || c == '_' || c == '+' || c == '-'
		if !ok {
			return false
		}
	}
	return true
}
