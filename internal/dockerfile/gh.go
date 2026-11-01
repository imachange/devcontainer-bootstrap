package dockerfile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// RealGH は、gh コマンドを実行する GH の実装です。
type RealGH struct{}

// LatestTag は、リポジトリの最新リリースのタグを返します。
func (RealGH) LatestTag(ctx context.Context, repo string) (string, error) {
	out, err := exec.CommandContext(ctx, "gh", "release", "view", "-R", repo, "--json", "tagName", "-q", ".tagName").Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Download は、リリースのアセットを dest に保存します。gh のエラー出力は、そのまま表示します。
func (RealGH) Download(ctx context.Context, repo, tag, asset, dest string) error {
	cmd := exec.CommandContext(ctx, "gh", "release", "download", tag, "-R", repo, "-p", asset, "-O", dest, "--clobber")
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Verify は、ファイルが、リリースの attestation と一致するかを確認します。
func (RealGH) Verify(ctx context.Context, repo, tag, file string) error {
	cmd := exec.CommandContext(ctx, "gh", "release", "verify-asset", tag, file, "-R", repo)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
