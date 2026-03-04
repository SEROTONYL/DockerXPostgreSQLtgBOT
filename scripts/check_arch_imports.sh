#!/usr/bin/env sh
set -eu

fail() {
  printf 'ERROR: %s\n' "$1" >&2
  exit 1
}

GOMOD_PATH="$(go env GOMOD 2>/dev/null || true)"
[ -n "$GOMOD_PATH" ] || fail "go env GOMOD is empty; run this script from a Go module root"
[ -f "go.mod" ] || fail "go.mod not found in current directory; run this script from module root"

MODULE_PATH="$(go list -m -f '{{.Path}}')"

check_bot_imports() {
  [ -d "internal/bot" ] || return 0

  forbidden_prefix="${MODULE_PATH}/internal/features"
  violations_file="$(mktemp)"
  trap 'rm -f "$violations_file"' EXIT INT TERM

  # Rule A applies to package internal/bot.
  go list -f '{{.ImportPath}} {{join .Imports " "}}' ./internal/bot | while IFS= read -r line; do
    pkg_name="$(printf '%s' "$line" | awk '{print $1}')"
    imports="$(printf '%s' "$line" | cut -d' ' -f2-)"
    for imp in $imports; do
      case "$imp" in
        "$forbidden_prefix"|"$forbidden_prefix"/*)
          printf '%s -> %s\n' "$pkg_name" "$imp" >>"$violations_file"
          ;;
      esac
    done
  done

  if [ -s "$violations_file" ]; then
    printf 'ERROR: internal/bot must not import internal/features/*\n' >&2
    sort -u "$violations_file" >&2
    exit 1
  fi
}

check_repo_like_imports() {
  patterns=""
  [ -d "internal/repo" ] && patterns="$patterns ./internal/repo/..."
  [ -d "internal/storage" ] && patterns="$patterns ./internal/storage/..."

  if [ -d "internal/db" ]; then
    for dir in internal/db/*; do
      [ -d "$dir" ] || continue
      base="$(basename "$dir")"
      [ "$base" = "postgres" ] && continue
      patterns="$patterns ./$dir/..."
    done
  fi

  [ -n "$(printf '%s' "$patterns" | tr -d '[:space:]')" ] || return 0

  forbidden_imports="
${MODULE_PATH}/internal/telegram
${MODULE_PATH}/internal/bot
github.com/go-telegram/bot
github.com/go-telegram/bot/models
github.com/go-telegram-bot-api/telegram-bot-api
github.com/go-telegram-bot-api/telegram-bot-api/v5
"

  violations_file="$(mktemp)"
  trap 'rm -f "$violations_file"' EXIT INT TERM

  for pkg in $patterns; do
    go list -f '{{.ImportPath}} {{join .Imports " "}}' "$pkg" | while IFS= read -r line; do
      pkg_name="$(printf '%s' "$line" | awk '{print $1}')"
      imports="$(printf '%s' "$line" | cut -d' ' -f2-)"
      for forbidden in $forbidden_imports; do
        [ -n "$forbidden" ] || continue
        if printf '%s\n' "$imports" | tr ' ' '\n' | grep -Fx "$forbidden" >/dev/null 2>&1; then
          printf '%s -> %s\n' "$pkg_name" "$forbidden" >>"$violations_file"
        fi
      done
    done
  done

  if [ -s "$violations_file" ]; then
    printf 'ERROR: repository/storage/db packages contain forbidden imports:\n' >&2
    sort -u "$violations_file" >&2
    exit 1
  fi
}

check_bot_imports
check_repo_like_imports

echo 'OK: architecture import rules passed'
