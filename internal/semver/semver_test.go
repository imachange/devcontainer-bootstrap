package semver

import "testing"

func TestParse(t *testing.T) {
	valid := []string{"v0.0.0", "v1.2.3", "v10.20.30", "v1.2.3-rc.1", "v1.2.3+build.5", "v1.2.3-rc.1+build", "v1.0.0-x-y.z"}
	for _, s := range valid {
		if _, ok := Parse(s); !ok {
			t.Errorf("Parse(%q): invalid, want valid", s)
		}
	}
	invalid := []string{
		"", "1.2.3", "v1", "v1.2", "v1.2.3.4", "v01.2.3", "v1.02.3",
		"v1.2.3-", "v1.2.3-01", "v1.2.3-rc..1", "v1.2.3+", "vx.y.z",
		"v1.2.3-rc_1", "v-1.2.3", " v1.2.3", "dev",
	}
	for _, s := range invalid {
		if _, ok := Parse(s); ok {
			t.Errorf("Parse(%q): valid, want invalid", s)
		}
	}
}

func mustParse(t *testing.T, s string) Version {
	t.Helper()
	v, ok := Parse(s)
	if !ok {
		t.Fatalf("Parse(%q): invalid", s)
	}
	return v
}

// semver 2.0.0 の仕様にある優先順位の例と、通常のバージョンの順序。
func TestCompareOrdering(t *testing.T) {
	ordered := []string{
		"v1.0.0-alpha", "v1.0.0-alpha.1", "v1.0.0-alpha.beta", "v1.0.0-beta",
		"v1.0.0-beta.2", "v1.0.0-beta.11", "v1.0.0-rc.1", "v1.0.0",
		"v1.0.1", "v1.1.0", "v2.0.0",
	}
	for i := 0; i+1 < len(ordered); i++ {
		a, b := mustParse(t, ordered[i]), mustParse(t, ordered[i+1])
		if got := Compare(a, b); got != -1 {
			t.Errorf("Compare(%s, %s) = %d, want -1", ordered[i], ordered[i+1], got)
		}
		if got := Compare(b, a); got != 1 {
			t.Errorf("Compare(%s, %s) = %d, want 1", ordered[i+1], ordered[i], got)
		}
		if got := Compare(a, a); got != 0 {
			t.Errorf("Compare(%s, %s) = %d, want 0", ordered[i], ordered[i], got)
		}
	}
}

func TestCompareIgnoresBuildMetadata(t *testing.T) {
	a, b := mustParse(t, "v1.0.0+a"), mustParse(t, "v1.0.0+b")
	if got := Compare(a, b); got != 0 {
		t.Errorf("Compare = %d, want 0", got)
	}
}
