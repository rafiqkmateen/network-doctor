#!/usr/bin/env bash
# Purpose: take the Windows pictures of the page after its check (ND-296) on GitHub's clean
# Windows machine, at the same sizes as the Mac and Linux ones, for a side-by-side look.
# Owner: Rafiq. Version 0.1.0 (2026-11-20: first build).
#
# Use (Git Bash on Windows): bash scripts/parity/windows-pictures.sh PROGRAM.exe OUTDIR
# The program runs `open` with its results in a scratch folder (NETDOCTOR_DIR). A stand-in for
# rundll32, found first on PATH, records the address instead of opening a browser, so only the
# headless browser loads the page.
set -uo pipefail

exe="$(cd "$(dirname "$1")" && pwd)/$(basename "$1")"
out="$2"
here="$(cd "$(dirname "$0")" && pwd)"
mkdir -p "$out/pictures" "$out/logs"
out="$(cd "$out" && pwd)"
results="$out/results.txt"
: > "$results"
fails=0
say() { echo "$*" | tee -a "$results"; }
check() {
  local what="$1"
  shift
  if "$@" > /dev/null 2>&1; then say "  PASS  $what"; else say "  FAIL  $what"; fails=$((fails + 1)); fi
}
waitfor() {
  local end=$((SECONDS + $1))
  shift
  while [ "$SECONDS" -lt "$end" ]; do
    if "$@" > /dev/null 2>&1; then return 0; fi
    sleep 1
  done
  return 1
}

scratch="${RUNNER_TEMP:-/tmp}/nd-windows"
rm -rf "$scratch"
mkdir -p "$scratch/results" "$scratch/shim"
urls="$scratch/urls.txt"
: > "$urls"
# cmd splits url.dll,FileProtocolHandler at the comma, so the whole line is recorded.
cat > "$scratch/shim/rundll32.cmd" << 'EOF'
@echo off
rem Records the address it was asked to open, instead of opening a browser.
echo %*>> "%ND_URLS%"
EOF

state="$scratch/results/open-page.json"
state_port() { sed -n 's/.*"port":"\([0-9]*\)".*/\1/p' "$state" 2> /dev/null; }
has_port() { [ -n "$(state_port)" ]; }

say "Network Doctor on Windows, pictures run $(date -u +%Y-%m-%dT%H%MZ)"
ND_URLS="$(cygpath -w "$urls")" NETDOCTOR_DIR="$(cygpath -w "$scratch/results")" \
  PATH="$scratch/shim:$PATH" "$exe" open > "$out/logs/open.log" 2>&1 &
pid=$!

if waitfor 30 has_port; then
  port="$(state_port)"
  say "  PASS  open-page.json names port $port"
  check "/api/health answers 200" test "$(curl -s -o /dev/null -m 10 -w '%{http_code}' "http://127.0.0.1:$port/api/health")" = 200
  if waitfor 10 grep -q "http://127.0.0.1:$port/" "$urls"; then
    say "  PASS  the address went to the rundll32 stand-in"
  else
    say "  NOTE  the rundll32 stand-in did not record the address; the port was read from open-page.json"
  fi
  plan="$scratch/plan.json"
  pics="$(cygpath -m "$out/pictures")"
  cat > "$plan" << EOF
{"url": "http://127.0.0.1:$port/", "steps": [
  {"size": [1280, 800]},
  {"wait": "/^Checked at/.test((document.getElementById('when') || {}).textContent || '')", "timeout": 180},
  {"shot": "$pics/windows-page.png"},
  {"size": [390, 844]},
  {"shot": "$pics/windows-page-phone.png"}
]}
EOF
  if node "$here/shot.mjs" "$(cygpath -w "$plan")" > "$out/logs/shot.log" 2>&1; then
    say "  PASS  pictures of the page after its check at 1280x800 and 390x844"
  else
    say "  FAIL  pictures of the page after its check"
    fails=$((fails + 1))
  fi
  sed 's/^/        /' "$out/logs/shot.log" | tee -a "$results"
else
  say "  FAIL  open-page.json did not name a port within 30 seconds"
  fails=$((fails + 1))
fi

kill "$pid" 2> /dev/null
taskkill //F //IM "$(basename "$exe")" > /dev/null 2>&1
say "== Done: $fails failed check(s)."
[ "$fails" -eq 0 ]
