#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

TAR_FILE="${1:-}"

if [ -z "${TAR_FILE}" ]; then
  echo "ERROR: tar.gz fayl nomi kerak." >&2
  exit 1
fi

if [ ! -f "${TAR_FILE}" ]; then
  echo "ERROR: ${TAR_FILE} topilmadi." >&2
  exit 1
fi

SUCCESS=0

cleanup() {
  rm -f "${TAR_FILE}"
  if [ "${SUCCESS}" -eq 0 ]; then
    echo ">>> Xatolik — docker-compose.yml tozalanmoqda"
    rm -f docker-compose.yml
  fi
  rm -f "$(readlink -f "$0")"
}
trap cleanup EXIT

echo ">>> Image yuklash: ${TAR_FILE}"
gunzip -c "${TAR_FILE}" | docker load

echo ">>> Container qayta ishga tushirilmoqda"
docker compose -f docker-compose.yml up -d

echo ">>> Eski (dangling) imagelarni tozalash"
docker image prune -f

echo ">>> Status"
docker compose -f docker-compose.yml ps

SUCCESS=1
