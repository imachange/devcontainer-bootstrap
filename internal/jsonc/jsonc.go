// Package jsonc は、devcontainer.json のような JSONC (コメントと末尾カンマを許す JSON) を
// 読むための最小の実装です。標準の JSON に直してから、encoding/json に渡します。
package jsonc

import (
	"bytes"
	"encoding/json"
	"errors"
)

// Standardize は、コメントと末尾カンマを取り除いた、標準の JSON を返します。
// 文字列の中の "//" や "/*"、"," は、そのまま残します。
func Standardize(src []byte) ([]byte, error) {
	src = bytes.TrimPrefix(src, []byte("\xef\xbb\xbf"))
	noComments, err := stripComments(src)
	if err != nil {
		return nil, err
	}
	return stripTrailingCommas(noComments), nil
}

// TopLevel は、トップレベルのオブジェクトのキーと値 (生の JSON) を返します。
func TopLevel(src []byte) (map[string]json.RawMessage, error) {
	std, err := Standardize(src)
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(std, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func stripComments(src []byte) ([]byte, error) {
	out := make([]byte, 0, len(src))
	inString := false
	for i := 0; i < len(src); i++ {
		c := src[i]
		if inString {
			out = append(out, c)
			if c == '\\' && i+1 < len(src) {
				i++
				out = append(out, src[i])
			} else if c == '"' {
				inString = false
			}
			continue
		}
		switch {
		case c == '"':
			inString = true
			out = append(out, c)
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			for i < len(src) && src[i] != '\n' {
				i++
			}
			if i < len(src) {
				out = append(out, '\n')
			}
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			end := bytes.Index(src[i+2:], []byte("*/"))
			if end < 0 {
				return nil, errors.New("jsonc: unterminated block comment")
			}
			for _, b := range src[i : i+2+end+2] {
				if b == '\n' {
					out = append(out, '\n')
				}
			}
			out = append(out, ' ')
			i += 2 + end + 1
		default:
			out = append(out, c)
		}
	}
	return out, nil
}

func stripTrailingCommas(src []byte) []byte {
	out := make([]byte, 0, len(src))
	inString := false
	for i := 0; i < len(src); i++ {
		c := src[i]
		if inString {
			out = append(out, c)
			if c == '\\' && i+1 < len(src) {
				i++
				out = append(out, src[i])
			} else if c == '"' {
				inString = false
			}
			continue
		}
		if c == '"' {
			inString = true
			out = append(out, c)
			continue
		}
		if c == ',' {
			j := i + 1
			for j < len(src) && isSpace(src[j]) {
				j++
			}
			if j < len(src) && (src[j] == '}' || src[j] == ']') {
				continue
			}
		}
		out = append(out, c)
	}
	return out
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}
