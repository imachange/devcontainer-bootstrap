#!/usr/bin/env bash
set -eu

mkdir -p dist stage
pkg=github.com/imachange/devcontainer-bootstrap/internal/version

for t in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
  os="${t%/*}"; arch="${t#*/}"; ext=""
  [ "$os" = windows ] && ext=".exe"
  name="devcontainer-bootstrap_${GITHUB_REF_NAME}_${os}_${arch}"
  mkdir -p "stage/${name}"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath \
    -ldflags "-s -w -X ${pkg}.Version=${GITHUB_REF_NAME}" \
    -o "stage/${name}/devcontainer-bootstrap${ext}" .
  # 生バイナリ (bootstrap.sh が gh で取得する)
  cp "stage/${name}/devcontainer-bootstrap${ext}" "dist/${name}${ext}"
  # アーカイブ (mise / aqua 向け。中身は実行ファイル 1 つだけ)
  if [ "$os" = windows ]; then
    (cd "stage/${name}" && zip -q "../../dist/${name}.zip" "devcontainer-bootstrap${ext}")
  else
    tar -C "stage/${name}" -czf "dist/${name}.tar.gz" devcontainer-bootstrap
  fi
done

(cd dist && sha256sum devcontainer-bootstrap_* > checksums.txt)
ls -l dist