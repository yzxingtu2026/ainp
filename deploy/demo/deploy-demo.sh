#!/usr/bin/env bash
set -Eeuo pipefail

if [[ $# -ne 1 || ! $1 =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "usage: $0 vMAJOR.MINOR.PATCH" >&2
  exit 2
fi

target_tag=$1
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
env_file=${AINP_ENV_FILE:-$script_dir/.env}
compose_file=${AINP_COMPOSE_FILE:-$script_dir/compose.yaml}
runtime_dir=${AINP_RUNTIME_DIR:-$script_dir/runtime}
backup_dir=$runtime_dir/backups
log_dir=$runtime_dir/logs
lock_file=$runtime_dir/deploy.lock

mkdir -p "$backup_dir" "$log_dir"
exec 9>"$lock_file"
if ! flock -n 9; then
  echo "another demo deployment is running" >&2
  exit 1
fi

log_file=$log_dir/$(date -u +%Y%m%dT%H%M%SZ)-${target_tag}.log
exec > >(tee -a "$log_file") 2>&1

for command in docker curl gzip python3 flock; do
  command -v "$command" >/dev/null || { echo "required command missing: $command"; exit 1; }
done
[[ -f $env_file ]] || { echo "environment file not found: $env_file"; exit 1; }
[[ -f $compose_file ]] || { echo "compose file not found: $compose_file"; exit 1; }

read_env() {
  local key=$1 fallback=${2:-}
  local value
  value=$(awk -F= -v key="$key" '$1 == key {sub(/^[^=]*=/, ""); print; exit}' "$env_file")
  printf '%s' "${value:-$fallback}"
}

write_tag() {
  local tag=$1 temporary
  temporary=$(mktemp "$script_dir/.env.XXXXXX")
  awk -F= -v tag="$tag" '
    BEGIN { found = 0 }
    $1 == "AINP_IMAGE_TAG" { print "AINP_IMAGE_TAG=" tag; found = 1; next }
    { print }
    END { if (!found) print "AINP_IMAGE_TAG=" tag }
  ' "$env_file" > "$temporary"
  chmod --reference="$env_file" "$temporary" 2>/dev/null || chmod 600 "$temporary"
  mv "$temporary" "$env_file"
}

compose=(docker compose --env-file "$env_file" -f "$compose_file")
old_tag=$(read_env AINP_IMAGE_TAG)
postgres_user=$(read_env POSTGRES_USER ainp)
postgres_db=$(read_env POSTGRES_DB ainp)
web_bind=$(read_env WEB_BIND_ADDR 127.0.0.1)
web_port=$(read_env WEB_PORT 8080)
health_url="http://${web_bind}:${web_port}/healthz"
health_retries=$(read_env HEALTH_RETRIES 30)
backup_retention_days=$(read_env BACKUP_RETENTION_DAYS 14)

verify_health() {
  local expected=$1 response
  for ((attempt = 1; attempt <= health_retries; attempt++)); do
    response=$(curl --fail --silent --max-time 5 "$health_url" 2>/dev/null || true)
    if RESPONSE="$response" EXPECTED="$expected" python3 - <<'PY'
import json, os, sys
try:
    data = json.loads(os.environ["RESPONSE"]).get("data", {})
except (KeyError, json.JSONDecodeError):
    sys.exit(1)
sys.exit(0 if data.get("status") == "ok" and data.get("version") == os.environ["EXPECTED"] else 1)
PY
    then
      echo "health verified: status=ok version=$expected"
      return 0
    fi
    sleep 5
  done
  return 1
}

rollback() {
  if [[ $old_tag =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "rolling application images back to $old_tag"
    write_tag "$old_tag"
    "${compose[@]}" up -d --remove-orphans
    verify_health "$old_tag" || echo "rollback health verification failed; inspect $log_file"
  else
    echo "no previous SemVer image is available for rollback"
  fi
}

echo "deploying $target_tag (previous=${old_tag:-none})"
write_tag "$target_tag"
if ! "${compose[@]}" pull api worker web; then
  write_tag "$old_tag"
  exit 1
fi

for image in backend web; do
  reference="ghcr.io/yzxingtu2026/ainp-${image}:${target_tag}"
  digest=$(docker image inspect --format '{{join .RepoDigests " "}}' "$reference")
  [[ -n $digest ]] || { echo "image digest missing: $reference"; rollback; exit 1; }
  echo "$image digest: $digest"
done

echo "starting database and redis"
if ! "${compose[@]}" up -d --wait postgres redis; then
  write_tag "$old_tag"
  exit 1
fi

backup_file=$backup_dir/${postgres_db}-$(date -u +%Y%m%dT%H%M%SZ)-before-${target_tag}.sql.gz
echo "creating database backup: $backup_file"
"${compose[@]}" exec -T postgres pg_dump -U "$postgres_user" "$postgres_db" | gzip -9 > "$backup_file"

echo "running forward-only migrations"
if ! "${compose[@]}" run --rm --no-deps --entrypoint /usr/local/bin/ainp-migrate api -config /app/config/config.yaml up; then
  echo "migration failed; new application images will not be started"
  write_tag "$old_tag"
  exit 1
fi

if ! "${compose[@]}" up -d --remove-orphans; then
  rollback
  exit 1
fi
if ! verify_health "$target_tag"; then
  echo "target health verification failed"
  rollback
  exit 1
fi

find "$backup_dir" -type f -name '*.sql.gz' -mtime "+$backup_retention_days" -delete
echo "deployment completed: $target_tag"
