# devcontainer-bootstrap

dev container 起動前に、**ホスト側**で実行される初期化ツールです。プロジェクトの `.devcontainer/script/bootstrap.sh` (シム) が、このバイナリを探して (または取得して) 実行します。

## コマンド

| コマンド | 説明 |
|---|---|
| `devcontainer-bootstrap` / `bootstrap [--root DIR]` | ホスト側の初期化 (下記)。`--root` の既定は、カレントディレクトリです (`initializeCommand` はリポジトリルートで実行されます) |
| `devcontainer-bootstrap version` | リリースタグだけを、標準出力に 1 行で出力する |
| `devcontainer-bootstrap select [--want <タグ>]` | `bin/` と PATH のどちらを使うかを判定する |
| `devcontainer-bootstrap help` | ヘルプを表示する |

## bootstrap の動作

次の順に実行します。どちらも、対象が無ければ何もしません。

1. `.devcontainer/devcontainer.json` に `build` があれば、公式の Dockerfile を取得する。
2. `.devcontainer/script/initialize.*` があれば、1 本だけ実行する。

Dockerfile の取得でエラーになったときは、`initialize.*` を実行せずに、終了コード 1 で止まります。

### 1. Dockerfile の取得

**対象**: `devcontainer.json` に `build` があり、`image` と `dockerComposeFile` が無いときだけです。`devcontainer.json` は JSONC (コメントと末尾カンマ) として読みます。

**置き場所**: `build.dockerfile` (省略時は `Dockerfile`) を、`.devcontainer/` からの相対パスとして解決します。`.devcontainer/` の外を指す値は、エラーにします。

**取得元**: builder リポジトリのリリース資産 `Dockerfile` です。

| 環境変数 | 既定値 | 説明 |
|---|---|---|
| `DEVBOOTSTRAP_BUILDER_REPO` | `imachange/devcontainer-builder` | 取得元リポジトリ (`owner/name`) |
| `DEVBOOTSTRAP_BUILDER_VERSION` | (未指定 = 最新リリース) | リリースタグ。チームで使うときは、固定を推奨します |

- 取得には `gh` を使い、`gh release verify-asset` で検証してから置きます。**検証に失敗したときは、必ずエラーにします。**
- builder のリリースには、`Dockerfile` という名前のアセットを添付してください。immutable releases を有効にしてください。
- `DEVBOOTSTRAP_BUILDER_VERSION` が未指定のときは、起動のたびに最新のタグを問い合わせます。固定すれば、取得済みの場合は `gh` を一切呼びません。

**上書きの条件**: 既存のファイルを上書きしてよいのは、次のどれかのときだけです。

| 状態 | 動作 |
|---|---|
| ファイルが無い | 取得して置く |
| 記録 (`<Dockerfile>.bootstrap`) があり、ファイルが取得したまま | リリースが変わったときだけ、取得して置き換える |
| 記録があり、ファイルが編集されている | 触らない (警告を出す) |
| 記録は無いが、git の ignore 対象 | 取得したものとして扱い、置き換える (旧 `bootstrap.sh` との互換) |
| 記録も無く、ignore 対象でもない | 利用者自身の Dockerfile として、触らない (何も出力しない) |

取得に失敗したとき (オフライン、`gh` が無いなど) は、既存の Dockerfile があれば、警告を出して続行します。既存が無ければ、エラーです。

`.devcontainer/.gitignore` に、次を入れてください。

```gitignore
Dockerfile
Dockerfile.*
```

`Dockerfile.bootstrap` は、取得したリリースとハッシュの記録です。

### 2. initialize.* の実行

`.devcontainer/script/` にある `initialize.<拡張子>` を、次の順に探し、**存在して、かつ実行できる最初の 1 本**だけを実行します。

| 優先 | ファイル | 実行方法 | 実行できる条件 |
|---|---|---|---|
| 1 | `initialize.ps1` | `pwsh -NoProfile -File` (Windows は `powershell` も。`-ExecutionPolicy Bypass` つき) | `pwsh` (Windows は `powershell` も) がある |
| 2 | `initialize.bat` | `cmd.exe /d /c` | Windows |
| 3 | `initialize.zsh` | `zsh <スクリプト>` | `zsh` がある |
| 4 | `initialize.bash` | `bash <スクリプト>` | `bash` がある |
| 5 | `initialize.sh` | `sh <スクリプト>` | `sh` がある |

- 作業ディレクトリは、リポジトリルートです。スクリプトは、ルートからの相対パスで渡すので、実行権限は要りません。
- 標準入力、標準出力、標準エラーは、そのまま引き継ぎます。終了コードは、そのまま返します。
- 見つかったが実行できなかったもの (インタプリタが無いなど) は、警告を出して、次の候補に進みます。存在するのに、1 本も実行できないときは、エラーです。
- Windows では、`System32` にある `bash.exe` (WSL のランチャー) は使いません。
- シンボリックリンクは、実体が `.devcontainer/script/` の中にあるものだけを実行します。
- `DEVBOOTSTRAP_SKIP_INITIALIZE=1` で、実行を省略できます。

## リリース (追加)

リリースには、次の 13 個のアセットを添付します。

| 種類 | 名前 | 用途 |
|---|---|---|
| 生バイナリ (6) | `devcontainer-bootstrap_<タグ>_<os>_<arch>[.exe]` | `bootstrap.sh` が `gh` で取得する |
| アーカイブ (6) | `devcontainer-bootstrap_<タグ>_<os>_<arch>.tar.gz` (Windows は `.zip`) | mise、aqua 向け。中身は実行ファイル 1 つだけ |
| チェックサム (1) | `checksums.txt` | `sha256sum` 形式。aqua、mise の検証向け |



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
