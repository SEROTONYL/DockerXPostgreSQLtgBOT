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

TMP_A="$(mktemp)"
TMP_B="$(mktemp)"
TMP_PKGS="$(mktemp)"
cleanup() {
  rm -f "$TMP_A" "$TMP_B" "$TMP_PKGS"
}
trap cleanup EXIT INT TERM

# Rule A: ./internal/bot/... must not import ${MODULE_PATH}/internal/features/*
if [ -d "internal/bot" ]; then
  go list -f '{{.ImportPath}} {{join .Imports " "}}' ./internal/bot/... |
    while IFS= read -r line; do
      pkg_name=$(printf '%s\n' "$line" | awk '{print $1}')
      imports=$(printf '%s\n' "$line" | cut -d' ' -f2-)
      for imp in $imports; do
        case "$imp" in
          "${MODULE_PATH}/internal/features"|"${MODULE_PATH}/internal/features"/*)
            printf '%s -> %s\n' "$pkg_name" "$imp" >>"$TMP_A"
            ;;
        esac
      done
    done
fi

if [ -s "$TMP_A" ]; then
  printf 'ERROR: Rule A violated: packages under ./internal/bot/... must not import %s/internal/features/*\n' "$MODULE_PATH" >&2
  sort -u "$TMP_A" >&2
  exit 1
fi

# Rule B: repo/storage/db packages must not import bot/telegram libraries.
for dir in internal/repo internal/storage; do
  if [ -d "$dir" ]; then
    go list "$dir/..." >>"$TMP_PKGS"
  fi
done

if [ -d "internal/db" ]; then
  find internal/db -mindepth 1 -maxdepth 1 -type d | while IFS= read -r dir; do
    base=$(basename "$dir")
    [ "$base" = "postgres" ] && continue
    go list "$dir/..." >>"$TMP_PKGS"
  done
fi

if [ -s "$TMP_PKGS" ]; then
  sort -u "$TMP_PKGS" -o "$TMP_PKGS"

  go list -f '{{.ImportPath}} {{join .Imports " "}}' $(cat "$TMP_PKGS") |
    while IFS= read -r line; do
      pkg_name=$(printf '%s\n' "$line" | awk '{print $1}')
      imports=$(printf '%s\n' "$line" | cut -d' ' -f2-)
      for imp in $imports; do
        case "$imp" in
          "${MODULE_PATH}/internal/telegram"|"${MODULE_PATH}/internal/bot"|"github.com/go-telegram/bot"|"github.com/go-telegram/bot/models"|"github.com/go-telegram-bot-api/telegram-bot-api"|"github.com/go-telegram-bot-api/telegram-bot-api/v5")
            printf '%s -> %s\n' "$pkg_name" "$imp" >>"$TMP_B"
            ;;
        esac
      done
    done
else
  printf "Skip rule B: no packages found\n"
fi

if [ -s "$TMP_B" ]; then
  printf 'ERROR: Rule B violated: repo/storage/db packages must not import bot or telegram adapters directly\n' >&2
  sort -u "$TMP_B" >&2
  exit 1
fi

printf 'OK: architecture import rules passed\n'
