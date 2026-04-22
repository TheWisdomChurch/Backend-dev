#!/usr/bin/env bash
set -euo pipefail

red() { printf "\033[31m%s\033[0m\n" "$*"; }
green() { printf "\033[32m%s\033[0m\n" "$*"; }
yellow() { printf "\033[33m%s\033[0m\n" "$*"; }

first_non_empty() {
  for key in "$@"; do
    val="${!key:-}"
    if [[ -n "${val// }" ]]; then
      printf "%s" "$val"
      return 0
    fi
  done
  return 1
}

boolish() {
  local v
  v="$(printf "%s" "${1:-}" | tr '[:upper:]' '[:lower:]' | xargs)"
  [[ "$v" == "1" || "$v" == "true" || "$v" == "yes" || "$v" == "on" ]]
}

check_tcp() {
  local host="$1"
  local port="$2"
  timeout 5 bash -c "cat < /dev/null > /dev/tcp/${host}/${port}" >/dev/null 2>&1
}

echo "== Email Diagnostics =="
echo "Timestamp: $(date -u '+%Y-%m-%dT%H:%M:%SZ')"
echo

disable_email="${DISABLE_EMAIL:-false}"
disable_otp="${DISABLE_OTP:-false}"

smtp_host="$(first_non_empty SMTP_HOST APP_SMTP_HOST MAIL_HOST || true)"
smtp_port="$(first_non_empty SMTP_PORT APP_SMTP_PORT MAIL_PORT || true)"
smtp_user="$(first_non_empty SMTP_USER APP_SMTP_USER SMTP_USERNAME MAIL_USERNAME || true)"
smtp_pass="$(first_non_empty SMTP_PASS APP_SMTP_PASS SMTP_PASSWORD MAIL_PASSWORD || true)"
smtp_from="$(first_non_empty SMTP_FROM MAIL_FROM || true)"
smtp_from_email="$(first_non_empty APP_SMTP_FROM_EMAIL SMTP_FROM_EMAIL || true)"
smtp_from_name="$(first_non_empty APP_SMTP_FROM_NAME SMTP_FROM_NAME MAIL_FROM_NAME || true)"
support_email="${APP_SUPPORT_EMAIL:-}"

brevo_key="${BREVO_API_KEY:-}"
brevo_from_email="${BREVO_FROM_EMAIL:-}"
brevo_from_name="${BREVO_FROM_NAME:-}"
brevo_base="${BREVO_BASE_URL:-https://api.brevo.com}"

if boolish "$disable_otp"; then
  red "DISABLE_OTP=true -> OTP login/password-reset emails are disabled."
fi
if boolish "$disable_email"; then
  red "DISABLE_EMAIL=true -> all outbound emails are disabled."
fi
if ! boolish "$disable_email" && ! boolish "$disable_otp"; then
  green "Email-related feature flags look enabled."
fi

echo
echo "Provider detection:"
if [[ -n "${smtp_host// }" ]]; then
  green "SMTP configured: host=${smtp_host} port=${smtp_port:-25}"
else
  yellow "SMTP not configured."
fi

if [[ -n "${brevo_key// }" || -n "${brevo_from_email// }" || -n "${brevo_from_name// }" ]]; then
  if [[ -n "${brevo_key// }" && -n "${brevo_from_email// }" ]]; then
    green "Brevo configured (base=${brevo_base})."
  else
    red "Brevo partially configured: BREVO_API_KEY and BREVO_FROM_EMAIL are both required."
  fi
else
  yellow "Brevo not configured."
fi

echo
echo "Sender identity:"
if [[ -z "${smtp_from// }" ]]; then
  if [[ -n "${smtp_from_email// }" ]]; then
    if [[ -n "${smtp_from_name// }" ]]; then
      smtp_from="${smtp_from_name} <${smtp_from_email}>"
    else
      smtp_from="${smtp_from_email}"
    fi
  elif [[ -n "${support_email// }" ]]; then
    smtp_from="${support_email}"
  elif [[ -n "${smtp_user// }" ]]; then
    smtp_from="${smtp_user}"
  fi
fi

if [[ -n "${smtp_host// }" && -z "${smtp_from// }" ]]; then
  red "No sender address resolved. Set SMTP_FROM or APP_SMTP_FROM_EMAIL (or APP_SUPPORT_EMAIL)."
else
  green "Resolved sender identity: ${smtp_from:-<none>}"
fi

echo
if [[ -n "${smtp_host// }" ]]; then
  p="${smtp_port:-25}"
  echo "Connectivity check:"
  if check_tcp "$smtp_host" "$p"; then
    green "TCP connectivity OK to ${smtp_host}:${p}"
  else
    red "Cannot connect to ${smtp_host}:${p} (network/DNS/firewall issue)."
  fi
fi

echo
echo "Auth check (SMTP remote hosts):"
if [[ -n "${smtp_host// }" ]]; then
  lc_host="$(printf "%s" "$smtp_host" | tr '[:upper:]' '[:lower:]')"
  if [[ "$lc_host" != "localhost" && "$lc_host" != "127.0.0.1" && "$lc_host" != "host.docker.internal" ]]; then
    if [[ -z "${smtp_user// }" || -z "${smtp_pass// }" ]]; then
      red "Remote SMTP host requires credentials. Set SMTP_USER/SMTP_PASS (or aliases)."
    else
      green "SMTP credentials look present for remote host."
    fi
  else
    yellow "Local SMTP relay detected; user/pass may be optional."
  fi
fi

echo
echo "Likely causes if users still don't get mail:"
echo "1) DNS/SPF/DKIM/DMARC not set for sender domain."
echo "2) Mail provider suppressing/bouncing due sender verification."
echo "3) Endpoint returns 500 while hiding internal cause (check API logs for 'Email delivery failed')."
echo "4) User account inactive/deactivated (OTP reset not sent for inactive users)."
echo "5) Recipient rate-limited (>10 emails/minute per address)."
