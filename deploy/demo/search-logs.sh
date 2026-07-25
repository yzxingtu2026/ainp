#!/usr/bin/env bash
set -Eeuo pipefail

if [[ $# -ne 2 || ! $1 =~ ^(request_id|trace_id)$ || ! $2 =~ ^[A-Za-z0-9._:-]{8,128}$ ]]; then
  echo "usage: $0 request_id|trace_id SAFE_ID" >&2
  exit 2
fi

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
env_file=${AINP_ENV_FILE:-$script_dir/.env}
configured_dir=$(awk -F= '$1 == "AINP_LOG_DIR" {sub(/^[^=]*=/, ""); print; exit}' "$env_file" 2>/dev/null || true)
log_dir=${configured_dir:-$script_dir/runtime/app-logs}
[[ $log_dir = /* ]] || log_dir=$script_dir/$log_dir
[[ -d $log_dir ]] || { echo "configured log directory not found" >&2; exit 1; }

pattern="\"$1\":\"$2\""
rg --fixed-strings "$pattern" "$log_dir" --glob '*.jsonl' --glob '*.jsonl.*' --glob '!*.gz' || true
while IFS= read -r -d '' archive; do
  gzip -cd -- "$archive" | rg --fixed-strings "$pattern" || true
done < <(find "$log_dir" -type f -name '*.gz' -print0)
