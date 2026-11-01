package jsonc

import (
	"encoding/json"
	"reflect"
	"testing"
)

func decode(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("decode %q: %v", s, err)
	}
	return v
}

func TestStandardize(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"行コメント", "{ // c\n \"a\": 1 }", `{"a": 1}`},
		{"ブロックコメント", `{ /* c */ "a": 1 /* d */ }`, `{"a": 1}`},
		{"複数行のブロックコメント", "{ /* a\nb\nc */ \"a\": 1 }", `{"a": 1}`},
		{"文字列中の //", `{"url": "https://example.com/x"}`, `{"url": "https://example.com/x"}`},
		{"文字列中の /*", `{"a": "/* not a comment */"}`, `{"a": "/* not a comment */"}`},
		{"エスケープした引用符の後の //", `{"a": "say \"hi\" // not comment"}`, `{"a": "say \"hi\" // not comment"}`},
		{"オブジェクトの末尾カンマ", `{"a": 1,}`, `{"a": 1}`},
		{"配列の末尾カンマ", `{"a": [1, 2, ], }`, `{"a": [1, 2]}`},
		{"カンマの後のコメントと閉じ括弧", "{\"a\": 1, // c\n}", `{"a": 1}`},
		{"文字列中のカンマと閉じ括弧", `{"a": ",}"}`, `{"a": ",}"}`},
		{"BOM", "\xef\xbb\xbf{\"a\": 1}", `{"a": 1}`},
		{"末尾の行コメント (改行なし)", `{"a": 1} // end`, `{"a": 1}`},
		{"入れ子", `{"build": {"dockerfile": "Dockerfile", /* x */ "args": {"A": "1",},},}`, `{"build": {"dockerfile": "Dockerfile", "args": {"A": "1"}}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Standardize([]byte(tt.src))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(decode(t, string(got)), decode(t, tt.want)) {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestStandardizeErrors(t *testing.T) {
	if _, err := Standardize([]byte(`{"a": 1 /* oops`)); err == nil {
		t.Error("閉じていないブロックコメントは、エラーになるはず")
	}
}

func TestTopLevel(t *testing.T) {
	m, err := TopLevel([]byte("{\n // c\n \"image\": \"x\",\n \"build\": {},\n}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m["image"]; !ok {
		t.Error("image が無い")
	}
	if _, ok := m["build"]; !ok {
		t.Error("build が無い")
	}
	if _, err := TopLevel([]byte(`{"a": }`)); err == nil {
		t.Error("不正な JSON は、エラーになるはず")
	}
	if _, err := TopLevel([]byte(`[1, 2]`)); err == nil {
		t.Error("トップレベルが配列なら、エラーになるはず")
	}
}
