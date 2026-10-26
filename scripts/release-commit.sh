#!/usr/bin/env bash
# release-commit.sh —— 只提交指定路径的提交助手。
#
# ## 为什么需要它
#
# 多个 agent 会话可能共享同一个工作树,也就共享**同一个 .git/index**。
# 而 `git commit`(不带 `--only`)提交的是**整个暂存区** ⇒
# 别人 `git add` 的文件会被一起带进你的提交。
#
# 本项目真实发生过:另一会话 `git add` 了它的 `docs/design/*.md`,
# 发版提交 `git show --name-only` 里就出现了这两个文件 —— 别人的半成品
# 被我的 tag 一起发布出去。
#
# `git commit --only <路径>` 只提交给定路径(**忽略暂存区里的其它内容**),
# 别人的 staged 文件原样留在 index 里,不受影响。
#
# ## 用法
#
#   scripts/release-commit.sh <提交信息文件> <路径>...   # 提交 + 自检
#   scripts/release-commit.sh --check <路径>...          # 只自检当前 HEAD
#
# 自检:本次提交的文件必须**全在**给定路径内;发现越界文件就
# **自动撤销**这次提交(`git reset --soft HEAD~1`,不丢任何内容)并非 0 退出。
set -euo pipefail

check_only=0
if [ "${1:-}" = "--check" ]; then
  check_only=1
  shift
fi

if [ "$#" -lt 1 ]; then
  echo "用法: $0 <提交信息文件> <路径>...  |  $0 --check <路径>..." >&2
  exit 2
fi

msg_file=""
if [ "$check_only" -eq 0 ]; then
  if [ "$#" -lt 2 ]; then
    echo "用法: $0 <提交信息文件> <路径>..." >&2
    exit 2
  fi
  msg_file="$1"
  shift
  if [ ! -f "$msg_file" ]; then
    echo "提交信息文件不存在: $msg_file" >&2
    exit 2
  fi
fi

paths=("$@")

# verify_head_in_paths 检查 HEAD 提交的文件是否全在允许路径内。
# 越界文件打印到 stderr,返回 1。
verify_head_in_paths() {
  local actual violations="" f p ok
  actual="$(git show --name-only --format= HEAD | sed '/^$/d')"
  for f in $actual; do
    ok=0
    for p in "${paths[@]}"; do
      case "$f" in
        "$p" | "$p"/*) ok=1; break ;;
      esac
    done
    [ "$ok" -eq 1 ] || violations="$violations $f"
  done
  if [ -n "$violations" ]; then
    echo "✗ HEAD 提交包含越界文件:$violations" >&2
    echo "  允许的路径: ${paths[*]}" >&2
    return 1
  fi
  return 0
}

if [ "$check_only" -eq 1 ]; then
  verify_head_in_paths || exit 1
  echo "✓ HEAD 提交只含指定路径"
  exit 0
fi

# --only:只提交给定路径,忽略暂存区里的其它内容(别的会话 add 的)。
git commit --only "${paths[@]}" -F "$msg_file"

if ! verify_head_in_paths; then
  echo "  ⇒ 已自动撤销这次提交(git reset --soft HEAD~1,内容未丢)" >&2
  git reset --soft HEAD~1
  exit 1
fi

echo "✓ 提交只含指定路径: $(git show --name-only --format= HEAD | sed '/^$/d' | tr '\n' ' ')"
