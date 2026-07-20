#!/usr/bin/env bash
# GitHub Action entrypoint for gsc-indexer.
# Maps the INPUT_* env vars GitHub injects to the CLI's real flags.
set -euo pipefail

CREDS_FILE=""
cleanup() { [ -n "$CREDS_FILE" ] && rm -f "$CREDS_FILE"; }
trap cleanup EXIT

# Write the service-account secret to a temp file. Never echo it.
if [ -n "${INPUT_SERVICE_ACCOUNT_JSON:-}" ]; then
  CREDS_FILE="$(mktemp)"
  chmod 600 "$CREDS_FILE"
  printf '%s' "$INPUT_SERVICE_ACCOUNT_JSON" > "$CREDS_FILE"
fi

args=()

if [ -n "$CREDS_FILE" ]; then
  args+=( -creds "$CREDS_FILE" )
fi
if [ -n "${INPUT_API_KEY:-}" ]; then
  args+=( -apikey "$INPUT_API_KEY" )
fi
if [ -n "${INPUT_SITE:-}" ]; then
  args+=( -site "$INPUT_SITE" )
fi
if [ -n "${INPUT_DELAY:-}" ]; then
  args+=( -delay "$INPUT_DELAY" )
fi

is_true() {
  case "${1:-}" in
    true|1|yes|y|on) return 0 ;;
    *) return 1 ;;
  esac
}
if is_true "${INPUT_QUIET:-}"; then args+=( -q ); fi
if is_true "${INPUT_DRY_RUN:-}"; then args+=( -dry-run ); fi
if is_true "${INPUT_JSON:-}"; then args+=( -json ); fi

if [ -n "${INPUT_COLOR:-}" ]; then
  args+=( -color "$INPUT_COLOR" )
fi
if [ -n "${INPUT_REPORT:-}" ]; then
  args+=( -report "$INPUT_REPORT" )
fi
if [ -n "${INPUT_DIFF:-}" ]; then
  args+=( -diff "$INPUT_DIFF" )
fi

# URLs: newline-separated input -> positional args (CLI dedups + expands .xml).
if [ -n "${INPUT_URLS:-}" ]; then
  while IFS= read -r u; do
    u="$(printf '%s' "$u" | tr -d '[:space:]')"
    [ -n "$u" ] && args+=( "$u" )
  done <<< "$INPUT_URLS"
fi

# Sitemap: a .xml URL passed positionally is auto-expanded by the CLI.
if [ -n "${INPUT_SITEMAP:-}" ]; then
  args+=( "$INPUT_SITEMAP" )
fi

exec gsc-indexer "${args[@]}"
