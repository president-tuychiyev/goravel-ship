#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

TAR_FILE="${1:-}"
IMAGE_NAME="${IMAGE_NAME:-app}"

if [ -z "${TAR_FILE}" ]; then
  echo "ERROR: tar.gz file name required." >&2
  exit 1
fi

if [ ! -f "${TAR_FILE}" ]; then
  echo "ERROR: ${TAR_FILE} not found." >&2
  exit 1
fi

SUCCESS=0

cleanup() {
  rm -f "${TAR_FILE}"
  if [ "${SUCCESS}" -eq 0 ]; then
    echo ">>> Error — cleaning up docker-compose.yml"
    rm -f docker-compose.yml
  fi
  rm -f "$(readlink -f "$0")"
}
trap cleanup EXIT

echo ">>> Loading image: ${TAR_FILE}"
gunzip -c "${TAR_FILE}" | docker load

echo ">>> Restarting container"
IMAGE_NAME="${IMAGE_NAME}" docker compose -f docker-compose.yml up -d

echo ">>> Cleaning up old (dangling) images"
docker image prune -f

echo ">>> Status"
docker compose -f docker-compose.yml ps

SUCCESS=1
