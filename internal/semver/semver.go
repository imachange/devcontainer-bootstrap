// Package semver は、semver 2.0.0 の優先順位の比較だけを行う最小実装。
// タグは `v` で始まる `vMAJOR.MINOR.PATCH[-prerelease]` の形式に限定。
package semver

import (
	"strconv"
	"strings"
)

// Version は、パース済みのバージョン。ビルドメタデータは比較に使わないので保持しない。
type Version struct {
	Major, Minor, Patch uint64
	Pre                 []string
}

// Parse は、`v1.2.3` や `v1.2.3-rc.1` の形式を解釈する。形式が不正なら ok=false。
func Parse(s string) (v Version, ok bool) {
	if !strings.HasPrefix(s, "v") {
		return Version{}, false
	}
	s = s[1:]
	if i := strings.IndexByte(s, '+'); i >= 0 {
		if !validBuild(s[i+1:]) {
			return Version{}, false
		}
		s = s[:i]
	}
	pre := ""
	hasPre := false
	if i := strings.IndexByte(s, '-'); i >= 0 {
		pre = s[i+1:]
		hasPre = true
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return Version{}, false
	}
	var nums [3]uint64
	for i, p := range parts {
		n, ok := parseNumeric(p)
		if !ok {
			return Version{}, false
		}
		nums[i] = n
	}
	v.Major, v.Minor, v.Patch = nums[0], nums[1], nums[2]
	if hasPre {
		ids := strings.Split(pre, ".")
		for _, id := range ids {
			if !validPreID(id) {
				return Version{}, false
			}
		}
		v.Pre = ids
	}
	return v, true
}

// Compare は、a が b より古ければ -1、同じなら 0、新しければ 1 を返す。
func Compare(a, b Version) int {
	if c := cmpUint(a.Major, b.Major); c != 0 {
		return c
	}
	if c := cmpUint(a.Minor, b.Minor); c != 0 {
		return c
	}
	if c := cmpUint(a.Patch, b.Patch); c != 0 {
		return c
	}
	return comparePre(a.Pre, b.Pre)
}

func comparePre(a, b []string) int {
	switch {
	case len(a) == 0 && len(b) == 0:
		return 0
	case len(a) == 0:
		return 1 // プレリリースの無い版の方が新しい
	case len(b) == 0:
		return -1
	}
	for i := 0; i < len(a) && i < len(b); i++ {
		if c := compareID(a[i], b[i]); c != 0 {
			return c
		}
	}
	return cmpUint(uint64(len(a)), uint64(len(b)))
}

func compareID(a, b string) int {
	an, aNum := parseNumeric(a)
	bn, bNum := parseNumeric(b)
	switch {
	case aNum && bNum:
		return cmpUint(an, bn)
	case aNum:
		return -1 // 数字だけの識別子は、英字を含むものより古い
	case bNum:
		return 1
	default:
		return strings.Compare(a, b)
	}
}

func cmpUint(a, b uint64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// parseNumeric は、先頭が 0 でない (または 0 だけの) 10 進数を解釈する。
func parseNumeric(s string) (uint64, bool) {
	if s == "" || (len(s) > 1 && s[0] == '0') {
		return 0, false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseUint(s, 10, 64)
	return n, err == nil
}

func isIdentChar(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '-'
}

func validPreID(s string) bool {
	if s == "" {
		return false
	}
	allDigits := true
	for i := 0; i < len(s); i++ {
		if !isIdentChar(s[i]) {
			return false
		}
		if s[i] < '0' || s[i] > '9' {
			allDigits = false
		}
	}
	return !(allDigits && len(s) > 1 && s[0] == '0')
}

func validBuild(s string) bool {
	if s == "" {
		return false
	}
	for _, id := range strings.Split(s, ".") {
		if id == "" {
			return false
		}
		for i := 0; i < len(id); i++ {
			if !isIdentChar(id[i]) {
				return false
			}
		}
	}
	return true
}
