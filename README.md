# devcontainer-bootstrap

dev container 起動前に、**ホスト側**で実行される初期化ツールです。プロジェクトの `.devcontainer/script/bootstrap.sh` (シム) が、このバイナリを探して (または取得して) 実行します。

## コマンド

| コマンド | 説明 |
|---|---|
| `devcontainer-bootstrap` / `bootstrap` | ホスト側の初期化の本体 (**未実装**。現状は何もせず成功を返す) |
| `devcontainer-bootstrap version` | リリースタグだけを、標準出力に 1 行で出力する |
| `devcontainer-bootstrap select [--want <タグ>]` | `bin/` と PATH のどちらを使うかを判定する |
| `devcontainer-bootstrap help` | ヘルプを表示する |

### `version` (契約)

- 出力は、**リリースタグだけ** (例: `v1.0.0`) です。`bootstrap.sh` と `select` が、この出力に依存します。
- ldflags が無いビルドは `dev` を出力します (`go install ...@v1.0.0` ではモジュールバージョンを使います)。

### `select` (契約)

`.devcontainer/bin/` に置かれたバイナリにだけ、シムが実行します。**判定だけ**を返し、`exec` はシムが行います。

- 標準出力: `bin` / `path` / `none` の **1 語だけ**。この 3 語は互換性のために固定します。
- 標準エラー: 判定の理由。
- 終了コード: 判定できたら 0。引数が不正なら 2。

| 入力 | 結果 |
|---|---|
| `--want` あり、自分のバージョンが一致 | `bin` |
| `--want` あり、自分は不一致で PATH が一致 | `path` |
| `--want` あり、どちらも不一致 | `none` |
| `--want` なし、PATH が無い、または実行できない | `bin` |
| `--want` なし、両方 semver で、PATH 側が**新しい** | `path` |
| `--want` なし、それ以外 (同じ、古い、semver として不正) | `bin` |

- `--want` の比較は、文字列の**完全一致**です (`1.0.0` と `v1.0.0` は別物)。
- PATH の候補は、`exec.LookPath` で探し、`version` を実行して (3 秒でタイムアウト) 調べます。`bin/` 自身と同じファイルは除外します。
- semver の比較は `internal/semver` (外部依存なし) で行います。

## 開発

```sh
go vet ./...
go test ./...
go build -ldflags "-X github.com/imachange/devcontainer-bootstrap/internal/version.Version=v0.0.0-dev" -o devcontainer-bootstrap .
```

## リリース

1. リポジトリの設定で、**immutable releases を有効にします** (`gh release verify-asset` が通るために必要です。過去の mutable なリリースには、後から attestation が付きません)。
2. `v*` のタグを push すると、`.github/workflows/release.yml` が、OS / arch ごとの生バイナリをビルドして、リリースを作成します。
   - アセット名: `devcontainer-bootstrap_<タグ>_<os>_<arch>[.exe]`
   - 下書きで作成してファイルを上げてから公開します (immutable releases は、公開後にファイルを追加できません)。

## Renovate の preset

`presets/devcontainer.json` は、利用側のリポジトリが `extends` するための共有設定です。

```json
{
  "extends": ["github>imachange/devcontainer-bootstrap//presets/devcontainer"]
}
```
