package initscript

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeScripts(t *testing.T, names ...string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ".devcontainer", "script")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("echo hi\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func lookPath(have map[string]string) func(string) (string, error) {
	return func(n string) (string, error) {
		if p, ok := have[n]; ok {
			return p, nil
		}
		return "", errors.New("not found")
	}
}

func TestFindOrder(t *testing.T) {
	all := []string{"initialize.ps1", "initialize.bat", "initialize.zsh", "initialize.bash", "initialize.sh"}
	tools := map[string]string{"pwsh": "/pwsh", "zsh": "/zsh", "bash": "/bash", "sh": "/sh", "cmd.exe": `C:\Windows\System32\cmd.exe`}

	tests := []struct {
		name   string
		goos   string
		files  []string
		have   map[string]string
		script string
	}{
		{"ps1 が最優先 (pwsh がある)", "linux", all, tools, "initialize.ps1"},
		{"pwsh が無ければ、Unix では bat も使えず zsh", "linux", all, map[string]string{"zsh": "/zsh", "bash": "/bash", "sh": "/sh"}, "initialize.zsh"},
		{"zsh が無ければ bash", "linux", all, map[string]string{"bash": "/bash", "sh": "/sh"}, "initialize.bash"},
		{"bash が無ければ sh", "linux", all, map[string]string{"sh": "/sh"}, "initialize.sh"},
		{"sh だけが存在", "darwin", []string{"initialize.sh"}, tools, "initialize.sh"},
		{"Windows: ps1 が無くて bat", "windows", []string{"initialize.bat", "initialize.sh"}, tools, "initialize.bat"},
		{"Windows: powershell があれば ps1", "windows", all, map[string]string{"powershell": `C:\ps\powershell.exe`, "cmd.exe": "cmd.exe"}, "initialize.ps1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := writeScripts(t, tt.files...)
			plan, err := Find(root, Deps{GOOS: tt.goos, LookPath: lookPath(tt.have)})
			if err != nil || plan == nil {
				t.Fatalf("plan=%v err=%v", plan, err)
			}
			if want := scriptDir + "/" + tt.script; plan.Script != want {
				t.Errorf("Script = %q, want %q", plan.Script, want)
			}
			if plan.Cmd.Dir != root {
				t.Errorf("Dir = %q, want %q", plan.Cmd.Dir, root)
			}
		})
	}
}

func TestFindCommands(t *testing.T) {
	root := writeScripts(t, "initialize.ps1")
	plan, err := Find(root, Deps{GOOS: "windows", LookPath: lookPath(map[string]string{"powershell": "PS"})})
	if err != nil {
		t.Fatal(err)
	}
	want := Command{Name: "PS", Args: []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", ".devcontainer/script/initialize.ps1"}, Dir: root}
	if !reflect.DeepEqual(plan.Cmd, want) {
		t.Errorf("windows ps1: got %+v, want %+v", plan.Cmd, want)
	}

	plan, err = Find(root, Deps{GOOS: "linux", LookPath: lookPath(map[string]string{"pwsh": "PWSH"})})
	if err != nil {
		t.Fatal(err)
	}
	want = Command{Name: "PWSH", Args: []string{"-NoProfile", "-File", ".devcontainer/script/initialize.ps1"}, Dir: root}
	if !reflect.DeepEqual(plan.Cmd, want) {
		t.Errorf("linux ps1: got %+v, want %+v", plan.Cmd, want)
	}

	root = writeScripts(t, "initialize.bat")
	plan, err = Find(root, Deps{GOOS: "windows", LookPath: lookPath(map[string]string{"cmd.exe": "CMD"})})
	if err != nil {
		t.Fatal(err)
	}
	want = Command{Name: "CMD", Args: []string{"/d", "/c", `.devcontainer\script\initialize.bat`}, Dir: root}
	if !reflect.DeepEqual(plan.Cmd, want) {
		t.Errorf("windows bat: got %+v, want %+v", plan.Cmd, want)
	}

	root = writeScripts(t, "initialize.sh")
	plan, err = Find(root, Deps{GOOS: "linux", LookPath: lookPath(map[string]string{"sh": "/bin/sh"})})
	if err != nil {
		t.Fatal(err)
	}
	want = Command{Name: "/bin/sh", Args: []string{".devcontainer/script/initialize.sh"}, Dir: root}
	if !reflect.DeepEqual(plan.Cmd, want) {
		t.Errorf("linux sh: got %+v, want %+v", plan.Cmd, want)
	}
}

func TestFindNothing(t *testing.T) {
	plan, err := Find(writeScripts(t), Deps{GOOS: "linux", LookPath: lookPath(nil)})
	if plan != nil || err != nil {
		t.Errorf("plan=%v err=%v, want nil, nil", plan, err)
	}
	// script ディレクトリ自体が無い場合
	plan, err = Find(t.TempDir(), Deps{GOOS: "linux", LookPath: lookPath(nil)})
	if plan != nil || err != nil {
		t.Errorf("plan=%v err=%v, want nil, nil", plan, err)
	}
}

func TestFindNoInterpreter(t *testing.T) {
	root := writeScripts(t, "initialize.bat", "initialize.zsh")
	plan, err := Find(root, Deps{GOOS: "linux", LookPath: lookPath(nil)})
	if plan != nil || err == nil {
		t.Errorf("plan=%v err=%v, want nil, error", plan, err)
	}
}

func TestFindSkipsWSLLauncher(t *testing.T) {
	root := writeScripts(t, "initialize.bash", "initialize.sh")
	have := map[string]string{"bash": `C:\Windows\System32\bash.exe`, "sh": `C:\Program Files\Git\usr\bin\sh.exe`}
	plan, err := Find(root, Deps{GOOS: "windows", LookPath: lookPath(have)})
	if err != nil || plan == nil {
		t.Fatalf("plan=%v err=%v", plan, err)
	}
	if plan.Script != scriptDir+"/initialize.sh" {
		t.Errorf("Script = %q, want initialize.sh", plan.Script)
	}
	if len(plan.Skipped) != 1 {
		t.Errorf("Skipped = %v", plan.Skipped)
	}
}

func TestFindRejectsNonRegular(t *testing.T) {
	root := writeScripts(t)
	// initialize.sh がディレクトリ
	if err := os.Mkdir(filepath.Join(root, ".devcontainer", "script", "initialize.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	plan, err := Find(root, Deps{GOOS: "linux", LookPath: lookPath(map[string]string{"sh": "/bin/sh"})})
	if plan != nil || err == nil {
		t.Errorf("plan=%v err=%v, want nil, error", plan, err)
	}
}

func TestFindSymlinks(t *testing.T) {
	root := writeScripts(t, "real.sh")
	dir := filepath.Join(root, ".devcontainer", "script")
	outside := filepath.Join(t.TempDir(), "outside.sh")
	if err := os.WriteFile(outside, []byte("echo x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deps := Deps{GOOS: "linux", LookPath: lookPath(map[string]string{"sh": "/bin/sh"})}

	// 外を指すシンボリックリンクは、使わない
	if err := os.Symlink(outside, filepath.Join(dir, "initialize.sh")); err != nil {
		t.Skipf("シンボリックリンクを作れない環境: %v", err)
	}
	if plan, err := Find(root, deps); plan != nil || err == nil {
		t.Errorf("外を指す: plan=%v err=%v, want nil, error", plan, err)
	}

	// 中を指すシンボリックリンクは、使える
	if err := os.Remove(filepath.Join(dir, "initialize.sh")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.sh", filepath.Join(dir, "initialize.sh")); err != nil {
		t.Skipf("シンボリックリンクを作れない環境: %v", err)
	}
	if plan, err := Find(root, deps); err != nil || plan == nil {
		t.Errorf("中を指す: plan=%v err=%v", plan, err)
	}
}
