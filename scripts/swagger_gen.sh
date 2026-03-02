#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${ROOT_DIR}/docs/generated"

mkdir -p "${OUT_DIR}"

run_swag() {
  go run github.com/swaggo/swag/cmd/swag@v1.8.12 init \
    -g handlers.go \
    -d internal/handlers/api,internal/models \
    -o "${OUT_DIR}" \
    --outputTypes json,yaml \
    --parseInternal \
    --parseDependency \
    --parseGoList=false
}

if [[ "${SWAG_SHOW_CONST_WARNINGS:-false}" == "true" ]]; then
  run_swag
else
  # Swag v1.8.x emits many false-positive const parsing warnings on modern Go stdlib.
  # Hide only that specific noise; keep all other output visible.
  run_swag 2>&1 | awk '!/warning: failed to evaluate const/ { print }'
fi

echo "Generated: ${OUT_DIR}/swagger.json and ${OUT_DIR}/swagger.yaml"
