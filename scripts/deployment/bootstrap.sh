set -eu
umask 077
work=$1
step() { python3 -u "$work/reporting.py" "$@"; }
. /etc/os-release
test "$ID" = ubuntu && test "$VERSION_ID" = 22.04
test "$(uname -m)" = x86_64
export DEBIAN_FRONTEND=noninteractive
exec 9>/var/lock/ainp-bootstrap.lock
step bootstrap-lock flock -w 120 9
sed -i 's#\(archive\|security\)\.ubuntu\.com#mirrors.cloud.aliyuncs.com#g' /etc/apt/sources.list

runtime=/opt/ainp-deploy-runtime-v1
if [ ! -f "$runtime/ready" ]; then
step apt-index sh "$work/apt.sh" update -qq
step system-packages sh "$work/apt.sh" install -y -qq python3-venv nginx curl ca-certificates gnupg tzdata libstdc++6
install -d -m 755 /usr/share/postgresql-common/pgdg
step postgresql-signing-key curl --fail --silent --show-error https://www.postgresql.org/media/keys/ACCC4CF8.asc -o /usr/share/postgresql-common/pgdg/apt.postgresql.org.asc
gpg --show-keys --with-colons /usr/share/postgresql-common/pgdg/apt.postgresql.org.asc | grep -q 'B97B0AFCAA1A47F044F244A07FCC7D46ACCC4CF8'
echo 'deb [signed-by=/usr/share/postgresql-common/pgdg/apt.postgresql.org.asc] https://apt.postgresql.org/pub/repos/apt jammy-pgdg main' >/etc/apt/sources.list.d/ainp-pgdg.list
chmod 644 /usr/share/postgresql-common/pgdg/apt.postgresql.org.asc /etc/apt/sources.list.d/ainp-pgdg.list
step apt-index sh "$work/apt.sh" update -qq
step postgresql-client sh "$work/apt.sh" install -y -qq postgresql-client-17
step python-venv python3 -m venv "$runtime"
step python-packages "$runtime/bin/pip" install --quiet oss2==2.19.1 'psycopg[binary]==3.2.10' PyYAML==6.0.2 redis==6.4.0
touch "$runtime/ready"
fi
step nginx systemctl enable --now nginx
"$runtime/bin/python" -u "$work/host.py" "$work/request.json"
