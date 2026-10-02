#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$SCRIPT_DIR"

REMOTE="${OTERO_DEPLOY_TARGET:-root@64.176.23.126}"
REMOTE_DIR="${OTERO_REMOTE_DIR:-/opt/oteroediciones}"
SERVICE="${OTERO_SERVICE:-oteroediciones}"
REMOTE_TMP="/tmp/otero-ediciones-release-$$"
LOCAL_BINARY="/tmp/otero-ediciones-release-$$"

cleanup() {
  rm -f "$LOCAL_BINARY"
}
trap cleanup EXIT INT TERM

echo "==> Running tests"
go test ./...
go vet ./...

echo "==> Building Tailwind CSS"
tailwindcss -i styles/input.css -o static/assets/app.css --minify

echo "==> Building Linux release binary"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -ldflags="-s -w" -o "$LOCAL_BINARY" .

echo "==> Uploading binary to $REMOTE"
scp "$LOCAL_BINARY" "$REMOTE:$REMOTE_TMP"

echo "==> Installing and restarting $SERVICE"
ssh "$REMOTE" "set -eu
  install -o root -g root -m 0755 '$REMOTE_TMP' '$REMOTE_DIR/otero-ediciones.new'
  mv '$REMOTE_DIR/otero-ediciones.new' '$REMOTE_DIR/otero-ediciones'
  rm -f '$REMOTE_TMP'
  systemctl restart '$SERVICE'
  systemctl is-active --quiet '$SERVICE'
  curl --fail --silent --show-error http://127.0.0.1:8091/healthz
"

echo "==> Deployment completed successfully"
