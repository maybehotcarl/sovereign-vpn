#!/bin/sh

set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
BLOCKED=0

pass() {
  printf 'PASS  %s\n' "$1"
}

warn() {
  printf 'WARN  %s\n' "$1"
}

fail() {
  printf 'FAIL  %s\n' "$1"
  BLOCKED=1
}

require_cmd() {
  if command -v "$1" >/dev/null 2>&1; then
    pass "found '$1'"
  else
    fail "missing required command '$1'"
  fi
}

require_cmd node
require_cmd npm
require_cmd ip
require_cmd docker

if [ -e /dev/net/tun ]; then
  pass "found /dev/net/tun"
else
  fail "/dev/net/tun is missing; WireGuard tunnels cannot come up"
fi

if command -v docker >/dev/null 2>&1; then
  if docker info >/dev/null 2>&1; then
    pass "docker daemon is reachable from this shell"
  else
    fail "docker daemon is not reachable from this shell"
  fi
fi

if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
  if docker compose version >/dev/null 2>&1; then
    pass "docker compose is available"
  else
    fail "docker compose is unavailable"
  fi
fi

if command -v wg >/dev/null 2>&1; then
  pass "host WireGuard tools are installed"
else
  warn "host WireGuard tools are not installed; the rehearsal stack expects containerized WireGuard"
fi

if [ -f "$ROOT_DIR/site-app/6529-zk-api/.env.local" ]; then
  pass "found zk-api env file at site-app/6529-zk-api/.env.local"
else
  fail "missing site-app/6529-zk-api/.env.local"
fi

if [ "$BLOCKED" -ne 0 ]; then
  printf '\nRehearsal prerequisites are not satisfied.\n'
  exit 1
fi

printf '\nRehearsal prerequisites look good.\n'
