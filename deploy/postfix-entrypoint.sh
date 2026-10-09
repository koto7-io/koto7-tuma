#!/bin/sh
# ==============================================================================
# Postfix entrypoint for Tuma
# Performs environment-variable substitution into /etc/postfix/main.cf
# before starting the Postfix daemon.
#
# Environment variables consumed:
#   POSTFIX_HOSTNAME   - EHLO hostname, e.g. mail.yourdomain.com
#   POSTFIX_DOMAIN     - Mail origin domain, e.g. yourdomain.com
#   POSTFIX_RELAYHOST  - Upstream smarthost (leave empty for direct delivery)
# ==============================================================================
set -e

# --------------------------------------------------------------------------
# Defaults
# --------------------------------------------------------------------------
POSTFIX_HOSTNAME="${POSTFIX_HOSTNAME:-mail.localhost}"
POSTFIX_DOMAIN="${POSTFIX_DOMAIN:-localhost}"
POSTFIX_RELAYHOST="${POSTFIX_RELAYHOST:-}"

echo "[postfix-entrypoint] hostname=$POSTFIX_HOSTNAME domain=$POSTFIX_DOMAIN relayhost='${POSTFIX_RELAYHOST}'"

# --------------------------------------------------------------------------
# Substitute env vars into main.cf
# The config file uses ${VAR} placeholders that are replaced here.
# --------------------------------------------------------------------------
sed -i \
  -e "s|\${POSTFIX_HOSTNAME}|${POSTFIX_HOSTNAME}|g" \
  -e "s|\${POSTFIX_DOMAIN}|${POSTFIX_DOMAIN}|g" \
  -e "s|\${POSTFIX_RELAYHOST}|${POSTFIX_RELAYHOST}|g" \
  /etc/postfix/main.cf

# --------------------------------------------------------------------------
# Ensure Postfix compatibility with Docker's /etc/hosts / /etc/resolv.conf
# --------------------------------------------------------------------------
postfix set-permissions 2>/dev/null || true
postfix check

# --------------------------------------------------------------------------
# Start Postfix in the foreground so Docker can manage the process
# --------------------------------------------------------------------------
exec postfix start-fg
