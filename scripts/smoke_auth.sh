#!/usr/bin/env bash
set -euo pipefail

# Manual smoke test for auth endpoints.
# Prereqs:
# - API running locally (default http://localhost:8080)
# - Postgres + Redis up
# - SMTP reachable if you want to validate password-reset email

BASE_URL="${BASE_URL:-http://localhost:8080}"
API="${BASE_URL%/}/api/v1"

EMAIL="${EMAIL:-admin+smoke@wisdomchurchhq.org}"
PASSWORD="${PASSWORD:-TestPass123!}"
NEW_PASSWORD="${NEW_PASSWORD:-NewPass123!}"
FIRST_NAME="${FIRST_NAME:-Smoke}"
LAST_NAME="${LAST_NAME:-Tester}"
ROLE="${ROLE:-admin}"

COOKIE_JAR="${COOKIE_JAR:-/tmp/wisdom_auth_cookies.txt}"
RESET_PAYLOAD_FILE="${RESET_PAYLOAD_FILE:-/tmp/wisdom_reset_payload.json}"

echo "==> Using API: ${API}"
echo "==> Using email: ${EMAIL}"
echo

echo "==> Register"
curl -sS -X POST "${API}/auth/register" \
  -H "Content-Type: application/json" \
  -d "{\"first_name\":\"${FIRST_NAME}\",\"last_name\":\"${LAST_NAME}\",\"email\":\"${EMAIL}\",\"password\":\"${PASSWORD}\",\"role\":\"${ROLE}\"}" \
  | tee /tmp/wisdom_register.json
echo

echo "==> Login (should succeed without OTP)"
curl -sS -X POST "${API}/auth/login" \
  -H "Content-Type: application/json" \
  -c "${COOKIE_JAR}" \
  -d "{\"email\":\"${EMAIL}\",\"password\":\"${PASSWORD}\",\"rememberMe\":true}" \
  | tee /tmp/wisdom_login.json
echo

echo "==> Get current user"
curl -sS -X GET "${API}/auth/me" \
  -b "${COOKIE_JAR}" \
  | tee /tmp/wisdom_me.json
echo

echo "==> Update profile (no-op change)"
curl -sS -X PATCH "${API}/auth/profile" \
  -H "Content-Type: application/json" \
  -b "${COOKIE_JAR}" \
  -d "{\"first_name\":\"${FIRST_NAME}\",\"last_name\":\"${LAST_NAME}\",\"email\":\"${EMAIL}\",\"username\":\"${FIRST_NAME,,}.${LAST_NAME,,}\"}" \
  | tee /tmp/wisdom_profile.json
echo

echo "==> Change password"
curl -sS -X POST "${API}/auth/change-password" \
  -H "Content-Type: application/json" \
  -b "${COOKIE_JAR}" \
  -d "{\"currentPassword\":\"${PASSWORD}\",\"newPassword\":\"${NEW_PASSWORD}\"}" \
  | tee /tmp/wisdom_change_password.json
echo

echo "==> Logout"
curl -sS -X POST "${API}/auth/logout" \
  -b "${COOKIE_JAR}" \
  | tee /tmp/wisdom_logout.json
echo

echo "==> Login with new password"
curl -sS -X POST "${API}/auth/login" \
  -H "Content-Type: application/json" \
  -c "${COOKIE_JAR}" \
  -d "{\"email\":\"${EMAIL}\",\"password\":\"${NEW_PASSWORD}\",\"rememberMe\":true}" \
  | tee /tmp/wisdom_login_new.json
echo

echo "==> Password reset request (check your email for OTP code)"
curl -sS -X POST "${API}/auth/password-reset/request" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"${EMAIL}\"}" \
  | tee "${RESET_PAYLOAD_FILE}"
echo

cat <<'EOF'
==> Next step (manual):
1. Check the email inbox for the reset OTP code.
2. Extract the "purpose" from the password-reset response JSON (saved in /tmp/wisdom_reset_payload.json).
3. Run the confirm command below after setting RESET_CODE and RESET_PURPOSE.

Example:
  RESET_CODE=123456 RESET_PURPOSE="password_reset:abc123" ./scripts/smoke_auth_confirm_reset.sh
EOF
