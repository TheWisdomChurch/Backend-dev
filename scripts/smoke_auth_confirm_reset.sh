#!/usr/bin/env bash
set -euo pipefail

# Confirm password reset (manual OTP step).
# Requires RESET_CODE and RESET_PURPOSE env vars.

BASE_URL="${BASE_URL:-http://localhost:8080}"
API="${BASE_URL%/}/api/v1"

EMAIL="${EMAIL:-admin+smoke@wisdomchurchhq.org}"
NEW_PASSWORD="${NEW_PASSWORD:-ResetPass123!}"

RESET_CODE="${RESET_CODE:-}"
RESET_PURPOSE="${RESET_PURPOSE:-}"

if [[ -z "${RESET_CODE}" || -z "${RESET_PURPOSE}" ]]; then
  echo "RESET_CODE and RESET_PURPOSE are required."
  exit 1
fi

curl -sS -X POST "${API}/auth/password-reset/confirm" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"${EMAIL}\",\"code\":\"${RESET_CODE}\",\"purpose\":\"${RESET_PURPOSE}\",\"newPassword\":\"${NEW_PASSWORD}\",\"confirmPassword\":\"${NEW_PASSWORD}\"}" \
  | tee /tmp/wisdom_reset_confirm.json
