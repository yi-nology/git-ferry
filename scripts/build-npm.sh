#!/usr/bin/env bash
# 构建 npm 包目录（含 skills + README），供 npm publish / 本地 pack 使用。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="${VERSION:-$(git -C "$ROOT" describe --tags --always --dirty 2>/dev/null || echo dev)}"
VERSION="${VERSION#v}"

OUT="${OUT:-$ROOT/npm-pkg}"
echo ">> building npm package v${VERSION} → ${OUT}"

rm -rf "$OUT"
mkdir -p "$OUT"
cp -R "$ROOT/npm/." "$OUT/"
cp "$ROOT/README.md" "$OUT/README.md"
rm -rf "$OUT/skills"
cp -R "$ROOT/skills" "$OUT/skills"

# 写入版本号
node -e '
const fs = require("fs");
const p = process.argv[1];
const pkg = JSON.parse(fs.readFileSync(p, "utf8"));
pkg.version = process.argv[2];
fs.writeFileSync(p, JSON.stringify(pkg, null, 2) + "\n");
' "$OUT/package.json" "$VERSION"

chmod +x "$OUT/bin/cli.js" "$OUT/bin/install-skills.js" "$OUT/scripts/install.js"

echo ">> npm package ready: $OUT"
echo ">> publish: cd $OUT && npm publish --access public"
