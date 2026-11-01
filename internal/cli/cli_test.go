package cli

import (
	"bytes"
	"context"
	"testing"

	"github.com/imachange/devcontainer-bootstrap/internal/selector"
	"github.com/imachange/devcontainer-bootstrap/internal/version"
)

func fakeDeps(pathVersion string, pathOK bool) selector.Deps {
	return selector.Deps{
		Executable: func() (string, error) { return "/repo/.devcontainer/bin/devcontainer-bootstrap", nil },
		LookPath: func(string) (string, error) {
			if !pathOK {
				return "", context.DeadlineExceeded
			}
			return "/usr/local/bin/devcontainer-bootstrap", nil
		},
		SameFile:   func(a, b string) (bool, error) { return false, nil },
		RunVersion: func(context.Context, string) (string, error) { return pathVersion, nil },
	}
}

func run(t *testing.T, self string, deps selector.Deps, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	old := version.Version
	version.Version = self
	defer func() { version.Version = old }()

	var out, errb bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errb, Deps: deps}
	code = app.Run(args)
	return code, out.String(), errb.String()
}

func TestVersionPrintsOnlyTheTag(t *testing.T) {
	code, out, _ := run(t, "v1.2.3", fakeDeps("", false), "version")
	if code != 0 || out != "v1.2.3\n" {
		t.Errorf("code=%d stdout=%q", code, out)
	}
}

func TestSelectOutputsOneWord(t *testing.T) {
	tests := []struct {
		name        string
		self        string
		pathVersion string
		pathOK      bool
		args        []string
		want        string
	}{
		{"未指定: PATH が新しい", "v1.0.0", "v1.1.0", true, []string{"select"}, "path\n"},
		{"未指定: PATH が無い", "v1.0.0", "", false, []string{"select"}, "bin\n"},
		{"固定: bin が一致", "v1.0.0", "v1.1.0", true, []string{"select", "--want", "v1.0.0"}, "bin\n"},
		{"固定: PATH が一致", "v0.9.0", "v1.0.0", true, []string{"select", "--want", "v1.0.0"}, "path\n"},
		{"固定: どちらも不一致", "v0.9.0", "v1.1.0", true, []string{"select", "--want", "v1.0.0"}, "none\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, errOut := run(t, tt.self, fakeDeps(tt.pathVersion, tt.pathOK), tt.args...)
			if code != 0 || out != tt.want {
				t.Errorf("code=%d stdout=%q (stderr=%q), want stdout=%q", code, out, errOut, tt.want)
			}
		})
	}
}

func TestSelectRejectsBadArguments(t *testing.T) {
	cases := [][]string{
		{"select", "--want", ""},
		{"select", "--bogus"},
		{"select", "extra"},
		{"select", "--want"},
	}
	for _, args := range cases {
		code, out, _ := run(t, "v1.0.0", fakeDeps("v1.0.0", true), args...)
		if code != 2 || out != "" {
			t.Errorf("%v: code=%d stdout=%q, want code=2 and empty stdout", args, code, out)
		}
	}
}

func TestUnknownCommand(t *testing.T) {
	if code, _, _ := run(t, "v1.0.0", fakeDeps("", false), "nope"); code != 2 {
		t.Errorf("code=%d, want 2", code)
	}
}

func TestDefaultIsBootstrap(t *testing.T) {
	if code, out, _ := run(t, "v1.0.0", fakeDeps("", false)); code != 0 || out != "" {
		t.Errorf("code=%d stdout=%q", code, out)
	}
}
