package dockerfile

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

type fakeGH struct {
	latest      string
	latestErr   error
	downloadErr error
	verifyErr   error
	content     string
	calls       []string
}

func (f *fakeGH) LatestTag(ctx context.Context, repo string) (string, error) {
	f.calls = append(f.calls, "latest")
	return f.latest, f.latestErr
}

func (f *fakeGH) Download(ctx context.Context, repo, tag, asset, dest string) error {
	f.calls = append(f.calls, "download:"+tag)
	if f.downloadErr != nil {
		return f.downloadErr
	}
	return os.WriteFile(dest, []byte(f.content), 0o644)
}

func (f *fakeGH) Verify(ctx context.Context, repo, tag, file string) error {
	f.calls = append(f.calls, "verify")
	return f.verifyErr
}

func (f *fakeGH) has(call string) bool {
	for _, c := range f.calls {
		if c == call {
			return true
		}
	}
	return false
}

func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const repo = "me/builder"

func setup(t *testing.T) (dir, path string) {
	dir = t.TempDir()
	return dir, filepath.Join(dir, "Dockerfile")
}

func TestSyncWritesNewFile(t *testing.T) {
	dir, path := setup(t)
	gh := &fakeGH{latest: "v1.0.0", content: "FROM scratch\n"}
	out, err := Sync(context.Background(), gh, Options{Repo: repo, Path: path})
	if err != nil || out.Result != Written || out.Tag != "v1.0.0" {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if got := read(t, path); got != "FROM scratch\n" {
		t.Errorf("content = %q", got)
	}
	if got := names(t, dir); len(got) != 2 || got[0] != "Dockerfile" || got[1] != "Dockerfile.bootstrap" {
		t.Errorf("files = %v (一時ファイルが残っている?)", got)
	}
	if !gh.has("latest") || !gh.has("download:v1.0.0") || !gh.has("verify") {
		t.Errorf("calls = %v", gh.calls)
	}
}

func TestSyncUpToDate(t *testing.T) {
	_, path := setup(t)
	gh := &fakeGH{latest: "v1.0.0", content: "A\n"}
	if _, err := Sync(context.Background(), gh, Options{Repo: repo, Path: path}); err != nil {
		t.Fatal(err)
	}
	gh.calls = nil
	out, err := Sync(context.Background(), gh, Options{Repo: repo, Path: path})
	if err != nil || out.Result != UpToDate {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if gh.has("download:v1.0.0") {
		t.Errorf("同じリリースなのに、再取得した: %v", gh.calls)
	}
}

func TestSyncPinnedSkipsLatestLookup(t *testing.T) {
	_, path := setup(t)
	gh := &fakeGH{content: "A\n"}
	if _, err := Sync(context.Background(), gh, Options{Repo: repo, Tag: "v2.0.0", Path: path}); err != nil {
		t.Fatal(err)
	}
	gh.calls = nil
	out, err := Sync(context.Background(), gh, Options{Repo: repo, Tag: "v2.0.0", Path: path})
	if err != nil || out.Result != UpToDate {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if len(gh.calls) != 0 {
		t.Errorf("固定版で取得済みなら、gh を一切呼ばないはず: %v", gh.calls)
	}
}

func TestSyncUpdatesWhenTagChanges(t *testing.T) {
	_, path := setup(t)
	gh := &fakeGH{content: "v1\n"}
	if _, err := Sync(context.Background(), gh, Options{Repo: repo, Tag: "v1.0.0", Path: path}); err != nil {
		t.Fatal(err)
	}
	gh.content = "v2\n"
	out, err := Sync(context.Background(), gh, Options{Repo: repo, Tag: "v2.0.0", Path: path})
	if err != nil || out.Result != Written {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if got := read(t, path); got != "v2\n" {
		t.Errorf("content = %q", got)
	}
}

func TestSyncLeavesUnmanagedFile(t *testing.T) {
	_, path := setup(t)
	if err := os.WriteFile(path, []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gh := &fakeGH{latest: "v1.0.0", content: "theirs\n"}
	out, err := Sync(context.Background(), gh, Options{Repo: repo, Path: path, IsIgnored: func(string) bool { return false }})
	if err != nil || out.Result != SkippedUnmanaged {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if got := read(t, path); got != "mine\n" {
		t.Errorf("自分の Dockerfile が書き換えられた: %q", got)
	}
	if len(gh.calls) != 0 {
		t.Errorf("gh を呼ばないはず: %v", gh.calls)
	}
}

func TestSyncOverwritesIgnoredFileWithoutRecord(t *testing.T) {
	_, path := setup(t)
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gh := &fakeGH{latest: "v1.0.0", content: "new\n"}
	out, err := Sync(context.Background(), gh, Options{Repo: repo, Path: path, IsIgnored: func(string) bool { return true }})
	if err != nil || out.Result != Written {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if got := read(t, path); got != "new\n" {
		t.Errorf("content = %q", got)
	}
}

func TestSyncLeavesModifiedFile(t *testing.T) {
	_, path := setup(t)
	gh := &fakeGH{content: "orig\n"}
	if _, err := Sync(context.Background(), gh, Options{Repo: repo, Tag: "v1.0.0", Path: path}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gh.calls = nil
	out, err := Sync(context.Background(), gh, Options{Repo: repo, Tag: "v2.0.0", Path: path})
	if err != nil || out.Result != SkippedModified || out.Message == "" {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if got := read(t, path); got != "edited\n" {
		t.Errorf("編集したファイルが書き換えられた: %q", got)
	}
}

func TestSyncDownloadFailure(t *testing.T) {
	dir, path := setup(t)

	// 既存が無ければ、エラー
	gh := &fakeGH{latest: "v1.0.0", downloadErr: errors.New("boom")}
	if _, err := Sync(context.Background(), gh, Options{Repo: repo, Path: path}); err == nil {
		t.Fatal("既存が無いなら、エラーになるはず")
	}
	if got := names(t, dir); len(got) != 0 {
		t.Errorf("files = %v", got)
	}

	// 既存があれば、警告して続行 (Kept)
	ok := &fakeGH{content: "orig\n"}
	if _, err := Sync(context.Background(), ok, Options{Repo: repo, Tag: "v1.0.0", Path: path}); err != nil {
		t.Fatal(err)
	}
	gh = &fakeGH{downloadErr: errors.New("boom")}
	out, err := Sync(context.Background(), gh, Options{Repo: repo, Tag: "v2.0.0", Path: path})
	if err != nil || out.Result != Kept || out.Message == "" {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if got := read(t, path); got != "orig\n" {
		t.Errorf("content = %q", got)
	}
	if got := names(t, dir); len(got) != 2 {
		t.Errorf("files = %v (一時ファイルが残っている?)", got)
	}
}

func TestSyncLatestLookupFailure(t *testing.T) {
	_, path := setup(t)
	gh := &fakeGH{latestErr: errors.New("offline")}
	if _, err := Sync(context.Background(), gh, Options{Repo: repo, Path: path}); err == nil {
		t.Fatal("既存が無ければ、エラー")
	}
	ok := &fakeGH{content: "orig\n"}
	if _, err := Sync(context.Background(), ok, Options{Repo: repo, Tag: "v1.0.0", Path: path}); err != nil {
		t.Fatal(err)
	}
	out, err := Sync(context.Background(), gh, Options{Repo: repo, Path: path})
	if err != nil || out.Result != Kept {
		t.Fatalf("out=%+v err=%v", out, err)
	}
}

func TestSyncVerificationFailureIsAlwaysAnError(t *testing.T) {
	dir, path := setup(t)
	ok := &fakeGH{content: "orig\n"}
	if _, err := Sync(context.Background(), ok, Options{Repo: repo, Tag: "v1.0.0", Path: path}); err != nil {
		t.Fatal(err)
	}
	gh := &fakeGH{content: "tampered\n", verifyErr: errors.New("attestation mismatch")}
	if _, err := Sync(context.Background(), gh, Options{Repo: repo, Tag: "v2.0.0", Path: path}); err == nil {
		t.Fatal("検証に失敗したら、エラーになるはず (既存があっても続行しない)")
	}
	if got := read(t, path); got != "orig\n" {
		t.Errorf("検証に失敗したのに、書き換えられた: %q", got)
	}
	if got := names(t, dir); len(got) != 2 {
		t.Errorf("files = %v (破棄されていない?)", got)
	}
}

func TestSyncRejectsBadInput(t *testing.T) {
	_, path := setup(t)
	gh := &fakeGH{}
	for _, o := range []Options{
		{Repo: "bad", Path: path},
		{Repo: "a/b/c", Path: path},
		{Repo: "me/builder", Tag: "../x", Path: path},
		{Repo: "me/builder", Tag: "-x", Path: path},
	} {
		if _, err := Sync(context.Background(), gh, o); err == nil {
			t.Errorf("%+v: エラーになるはず", o)
		}
	}
	if len(gh.calls) != 0 {
		t.Errorf("gh を呼ばないはず: %v", gh.calls)
	}
}

func TestSyncCreatesParentDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "docker", "Dockerfile")
	gh := &fakeGH{latest: "v1.0.0", content: "A\n"}
	if _, err := Sync(context.Background(), gh, Options{Repo: repo, Path: path}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, path); got != "A\n" {
		t.Errorf("content = %q", got)
	}
}

func TestValidators(t *testing.T) {
	for _, s := range []string{"imachange/devcontainer-builder", "a/b", "A_b.c/d+e-f"} {
		if !ValidRepo(s) {
			t.Errorf("ValidRepo(%q) = false", s)
		}
	}
	for _, s := range []string{"", "a", "/b", "a/", "a/b/c", "a b/c", "-a/b", "a/..", "a/b;c"} {
		if ValidRepo(s) {
			t.Errorf("ValidRepo(%q) = true", s)
		}
	}
	for _, s := range []string{"v1.0.0", "1.0.0", "v1.0.0-rc.1", "v1.0.0+b"} {
		if !ValidTag(s) {
			t.Errorf("ValidTag(%q) = false", s)
		}
	}
	for _, s := range []string{"", "-x", "../x", "a/b", "a b", "v1\n"} {
		if ValidTag(s) {
			t.Errorf("ValidTag(%q) = true", s)
		}
	}
}
