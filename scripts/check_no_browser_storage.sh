#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if rg -n "localStorage|sessionStorage|window\.localStorage|window\.sessionStorage" -S . \
  -g '!scripts/check_no_browser_storage.sh' \
  -g '!.git/**'; then
  echo "[FAIL] Browser storage usage detected. Keep auth/session state backend-driven (HttpOnly cookies + server session rules)."
  exit 1
fi

echo "[OK] No browser storage usage detected."
