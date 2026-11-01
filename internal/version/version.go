// Package version は、このバイナリのリリースタグを保持します。
package version

import "runtime/debug"

// Version は、リリース時に次のように埋め込みます。
//
//    go build -ldflags "-X github.com/imachange/container-bootstrap/internal/version.Version=v1.0.0"
var Version = "dev"

// Get は、`version` コマンドが出力するタグを返します。
// ldflags が無い場合 (go install など) は、ビルド情報のモジュールバージョンを使います。
func Get() string {
	if Version != "dev" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return Version
}
