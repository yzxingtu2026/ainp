set -eu
apt_get() {
  i=0
  while :; do
    if output=$(apt-get "$@" 2>&1); then printf '%s\n' "$output"; return 0; else status=$?; fi
    case "$output" in
      *"Could not get lock"*|*"Unable to acquire "*lock*|*"Could not open lock file"*) ;;
      *) printf '%s\n' "$output" >&2; return "$status" ;;
    esac
    if [ "$i" -ge 120 ]; then
      printf '%s\n' "$output" >&2
      printf 'AINP_APT_LOCK_TIMEOUT: reconcile unattended-upgrades; retry 600s\n' >&2
      return 75
    fi
    i=$((i+1));sleep 5
  done
}
apt_get "$@"
