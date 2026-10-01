#!/usr/bin/env bash
# 本地打包自测：把当前二进制塞进 npm 包并 npm pack，验证 wrapper 可执行。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="${VERSION:-$(git -C "$ROOT" describe --tags --always --dirty 2>/dev/null || echo dev)}"
VERSION="${VERSION#v}"

echo ">> building CLI binary..."
make -C "$ROOT" build-cli

OUT="$ROOT/npm-pkg"
bash "$ROOT/scripts/build-npm.sh"

# 把本机二进制放进包，跳过 postinstall 下载
case "$(uname -s)" in
  Darwin) BIN="gitferry" ;;
  Linux)  BIN="gitferry" ;;
  MINGW*|MSYS*) BIN="gitferry.exe" ;;
  *) BIN="gitferry" ;;
esac
cp "$ROOT/output/gitferry" "$OUT/bin/$BIN"
chmod +x "$OUT/bin/$BIN"

echo ">> npm pack..."
NPM_CMD="npm"
if ! command -v npm >/dev/null 2>&1; then
  if [ -n "${MIMO_NPM:-}" ] && [ -n "${MIMO_NODE:-}" ]; then
    NPM_CMD=("$MIMO_NODE" "$MIMO_NPM")
  elif command -v node >/dev/null 2>&1 && [ -f "$(dirname "$(command -v node)")/../lib/node_modules/npm/bin/npm-cli.js" ]; then
    NPM_CMD=(node "$(dirname "$(command -v node)")/../lib/node_modules/npm/bin/npm-cli.js")
  else
    echo "!! npm not found; skip npm pack (package dir is ready at $OUT)"
    ls -la "$OUT"
    exit 0
  fi
else
  NPM_CMD=(npm)
fi
(cd "$OUT" && "${NPM_CMD[@]}" pack)

echo ">> done. tarball in $OUT/"
ls -la "$OUT"/*.tgz
