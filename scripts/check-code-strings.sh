#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
failed=0

check() {
  description=$1
  pattern=$2
  shift 2
  if rg -n "$pattern" "$@"; then
    printf '%s\n' "code-string check failed: $description" >&2
    failed=1
  fi
}

check "status comparisons must use typed constants" '(Status|status)[[:space:]]*(==|!=)[[:space:]]*"[a-z_]+"' "$root/platform" -g '*.go' -g '!tests/**'
check "do not introduce duplicate enabled/disabled const sets" 'StatusEnabled[[:space:]]*=|StatusDisabled[[:space:]]*=' "$root/platform" -g '*.go'
check "business pages must not declare enabled/disabled option values" "value:[[:space:]]*['\"](enabled|disabled)['\"]" "$root/frontend/src/views" -g '*.ts' -g '*.vue'

exit "$failed"
