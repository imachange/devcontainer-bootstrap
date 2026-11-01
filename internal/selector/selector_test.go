package selector

import (
	"context"
	"errors"
	"testing"
)

func TestDecide(t *testing.T) {
	ok := func(v string) Candidate { return Candidate{Version: v, OK: true} }
	none := Candidate{}
	tests := []struct {
		name         string
		want, self   string
		path         Candidate
		expect       Decision
	}{
		{"固定: bin が一致", "v1.0.0", "v1.0.0", ok("v0.9.0"), Bin},
		{"固定: bin と PATH の両方が一致", "v1.0.0", "v1.0.0", ok("v1.0.0"), Bin},
		{"固定: bin が不一致で PATH が一致", "v1.0.0", "v0.9.0", ok("v1.0.0"), Path},
		{"固定: どちらも不一致", "v1.0.0", "v0.9.0", ok("v1.1.0"), None},
		{"固定: PATH が無く bin が不一致", "v1.0.0", "v0.9.0", none, None},
		{"固定: v の有無は別物として扱う", "v1.0.0", "1.0.0", none, None},
		{"未指定: PATH が無い", "", "v1.0.0", none, Bin},
		{"未指定: PATH が新しい", "", "v1.0.0", ok("v1.1.0"), Path},
		{"未指定: PATH が古い", "", "v1.1.0", ok("v1.0.0"), Bin},
		{"未指定: 同じ版", "", "v1.0.0", ok("v1.0.0"), Bin},
		{"未指定: PATH がプレリリースで古い", "", "v1.0.0", ok("v1.0.0-rc.1"), Bin},
		{"未指定: PATH が正式版で bin がプレリリース", "", "v1.0.0-rc.1", ok("v1.0.0"), Path},
		{"未指定: bin が dev で比較できない", "", "dev", ok("v9.9.9"), Bin},
		{"未指定: PATH の版が不正", "", "v1.0.0", ok("garbage"), Bin},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason := Decide(tt.want, tt.self, tt.path)
			if got != tt.expect {
				t.Errorf("Decide(%q, %q, %+v) = %s (%s), want %s", tt.want, tt.self, tt.path, got, reason, tt.expect)
			}
			if reason == "" {
				t.Error("reason が空")
			}
		})
	}
}

func newDeps() Deps {
	return Deps{
		Executable: func() (string, error) { return "/repo/.devcontainer/bin/devcontainer-bootstrap", nil },
		LookPath:   func(string) (string, error) { return "/usr/local/bin/devcontainer-bootstrap", nil },
		SameFile:   func(a, b string) (bool, error) { return false, nil },
		RunVersion: func(context.Context, string) (string, error) { return "v1.2.3", nil },
	}
}

func TestProbe(t *testing.T) {
	ctx := context.Background()

	if c, _ := Probe(ctx, newDeps(), "x"); !c.OK || c.Version != "v1.2.3" {
		t.Errorf("通常: got %+v", c)
	}

	d := newDeps()
	d.LookPath = func(string) (string, error) { return "", errors.New("not found") }
	if c, _ := Probe(ctx, d, "x"); c.OK {
		t.Errorf("PATH に無い: got %+v", c)
	}

	d = newDeps()
	d.SameFile = func(a, b string) (bool, error) { return true, nil }
	if c, _ := Probe(ctx, d, "x"); c.OK {
		t.Errorf("bin 自身は除外される: got %+v", c)
	}

	d = newDeps()
	d.RunVersion = func(context.Context, string) (string, error) { return "", errors.New("boom") }
	if c, _ := Probe(ctx, d, "x"); c.OK {
		t.Errorf("version が失敗: got %+v", c)
	}
}

func TestRun(t *testing.T) {
	d := newDeps()
	d.RunVersion = func(context.Context, string) (string, error) { return "v1.3.0", nil }
	got, notes := Run(context.Background(), d, "x", "v1.2.0", "")
	if got != Path {
		t.Errorf("got %s, want %s", got, Path)
	}
	if len(notes) != 2 {
		t.Errorf("notes = %v", notes)
	}
}
