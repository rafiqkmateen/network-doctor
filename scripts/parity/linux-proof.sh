#!/usr/bin/env bash
# Purpose: prove the Linux setup on GitHub's clean Linux machine (ND-270, ND-271, ND-278,
# ND-283, ND-286, ND-290, ND-291, ND-295 to ND-297, ND-309 to ND-312): install each of the four
# choices with the real program, check every file, icon and job, open the page with the launcher's
# own command, take pictures, press the launcher twice, run the weekly job and the alerts helper
# through the system that holds them, then uninstall: keeping the results with --keep-results,
# and with no switch and with the old --delete-results, which both delete everything (before one
# plain removal an undo notes file that cannot be read is planted, to prove the undo step runs first
# and keeps the notes, ND-316); and a second pass with no user systemd, to prove the crontab fallback.
# Owner: Rafiq. Version 0.6.1 (0.6.1, 2026-11-23: the planted notes cannot be read, since a changed
# setting is now settled at removal; 0.6.0, 2026-11-23: the planted undo note; 0.5.0, 2026-11-23: a plain silent uninstall deletes the results,
# --keep-results keeps them, the old --delete-results is checked to still work; 2026-11-20, last: every phone-width picture must fit 390 wide, and
# the finish screen's guide button is pressed and pictured (ND-279); 0.3.0, 2026-11-20 later: the runner's XDG folders taken out of the test
# user's systemd, the weekly checks follow the program's own result line, evidence printed when a
# unit step fails; 0.2.0, 2026-11-20: launchers started by gio launch from /, the removed
# notice pictured, the test alert, a plain stop when user systemd cannot be reached; 0.1.0: first
# build).
#
# Use: bash scripts/parity/linux-proof.sh SETUP.tar.gz OUTDIR
# Run it only on a throwaway machine: it makes a new user account and changes its files.
#
# Why a new user and not just a new HOME: the person's own systemd reads its units from its own
# home folder and runs them with that home, so a made-up HOME would hide the timer and the
# service from it. A new account has a home that is truly empty and its own user systemd.
set -uo pipefail # not -e: every check runs and is counted

tarball="$(cd "$(dirname "$1")" && pwd)/$(basename "$1")"
out="$2"
here="$(cd "$(dirname "$0")" && pwd)"
mkdir -p "$out/pictures" "$out/logs"
out="$(cd "$out" && pwd)"
results="$out/results.txt"
: > "$results"
fails=0

say() { echo "$*" | tee -a "$results"; }
pass() { say "  PASS  $1"; }
fail() { say "  FAIL  $1"; fails=$((fails + 1)); }
note() { say "  NOTE  $1"; }
# check "what" command...: PASS when the command succeeds.
check() {
  local what="$1"
  shift
  if "$@" > /dev/null 2>&1; then pass "$what"; else fail "$what"; return 1; fi
}
# ut: a test on the test user's files (its home is closed to other users).
ut() { sudo test "$@"; }
not() { ! "$@"; }
# waitfor SECONDS command...: true once the command succeeds, false when time runs out.
waitfor() {
  local end=$((SECONDS + $1))
  shift
  while [ "$SECONDS" -lt "$end" ]; do
    if "$@" > /dev/null 2>&1; then return 0; fi
    sleep 1
  done
  return 1
}

# ---------------------------------------------------------------------------------------------
# The test user, with its own user systemd kept running (linger).
tu=ndparity
sudo useradd -m -s /bin/bash "$tu"
th="$(getent passwd "$tu" | cut -d: -f6)"
tuid="$(id -u "$tu")"
sudo loginctl enable-linger "$tu"
# Linger starts the user manager (user@UID.service) without a log-in. systemctl --user then talks
# to it over /run/user/UID/bus (dbus-user-session) or systemd/private. Every later step needs it,
# so the run stops here with a plain message when it cannot be reached.
sudo systemctl start "user@$tuid.service" 2> /dev/null
if waitfor 60 sudo test -S "/run/user/$tuid/bus"; then
  pass "the test user's session bus is there: /run/user/$tuid/bus"
else
  fail "the test user's session bus /run/user/$tuid/bus did not appear within 60 seconds"
fi

# Small helper programs every user can run: a browser stand-in that only records the address,
# and a systemctl stand-in that always fails (for the pass with no user systemd).
tools=/tmp/nd-tools
nosd=/tmp/nd-nosd
mkdir -p "$tools" "$nosd"
urls="$tools/urls.txt"
cat > "$tools/record-url" << EOF
#!/bin/sh
# Records the address it was asked to open, instead of opening a browser.
printf '%s\n' "\$1" >> "$urls"
EOF
cp "$tools/record-url" "$tools/xdg-open"
# A notify-send stand-in for the alerts helper: it records its arguments and succeeds.
screen_stub=/tmp/nd-screen
screen_calls="$screen_stub/calls.txt"
mkdir -p "$screen_stub"
cat > "$screen_stub/notify-send" << EOF
#!/bin/sh
# Records the screen alert it was asked to show, as one line, and reports success.
printf '%s\n' "\$*" >> "$screen_calls"
EOF
: > "$screen_calls"
chmod 0755 "$screen_stub" "$screen_stub/notify-send"
chmod 0666 "$screen_calls"
cat > "$nosd/systemctl" << 'EOF'
#!/bin/sh
# Stands in for systemctl on a computer with no user systemd.
echo "System has not been booted with systemd as init system (PID 1). Can't operate." >&2
exit 1
EOF
chmod 0755 "$tools" "$nosd" "$tools/record-url" "$tools/xdg-open" "$nosd/systemctl"
: > "$urls"
chmod 0666 "$urls"

base_path="$tools:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
upath="$base_path"
# as_user runs a command as the test user with a clean environment: no XDG folders set, only
# the runtime folder its systemd needs.
as_user() {
  sudo -u "$tu" -H env -i HOME="$th" USER="$tu" LOGNAME="$tu" SHELL=/bin/bash LANG=C.UTF-8 \
    PATH="$upath" XDG_RUNTIME_DIR="/run/user/$tuid" \
    DBUS_SESSION_BUS_ADDRESS="unix:path=/run/user/$tuid/bus" BROWSER="$tools/record-url" "$@"
}

if as_user systemctl --user show-environment > "$out/logs/user-systemd.log" 2>&1; then
  pass "systemctl --user reaches the test user's own systemd (sudo -u, XDG_RUNTIME_DIR, DBUS_SESSION_BUS_ADDRESS)"
else
  fail "STOPPED at the user systemd step: systemctl --user, run as $tu with XDG_RUNTIME_DIR=/run/user/$tuid and DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/$tuid/bus after enable-linger, could not reach the user manager"
  sed 's/^/        /' "$out/logs/user-systemd.log" | tee -a "$results"
  sudo systemctl status "user@$tuid.service" --no-pager 2>&1 | sed 's/^/        /' | tee -a "$results"
  say "== Done: stopped early, $fails failed check(s)."
  exit 1
fi
# GitHub's machine lists the runner account's own settings in /etc/environment, and the user
# manager reads that file: run 44698247327 showed XDG_CONFIG_HOME=/home/runner/.config in the test
# user's systemd, so the weekly job and the alerts helper looked for the settings in the runner's
# folder ("permission denied") while the install had used the test user's own. A real desktop
# never hands one person's folders to another, so the test user's manager is given the plain
# values a fresh account has: each XDG_*_HOME set to the test user's own folder (set, not only
# taken out, so a value read at the manager's start cannot come back after a reload), and a PATH
# with no runner folders.
leaks="$(as_user systemctl --user show-environment | sed -n 's/^\(XDG_[A-Z]*_HOME\)=.*/\1/p')"
if [ -n "$leaks" ]; then
  note "set by the runner image in the test user's systemd: $(echo $leaks)"
fi
as_user systemctl --user set-environment "XDG_CONFIG_HOME=$th/.config" "XDG_DATA_HOME=$th/.local/share" \
  "XDG_CACHE_HOME=$th/.cache" "XDG_STATE_HOME=$th/.local/state" \
  "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
# clean_manager_env: the manager has the test user's HOME, and every XDG_*_HOME it holds lies in
# that home.
clean_manager_env() {
  local envs
  envs="$(as_user systemctl --user show-environment)" || return 1
  printf '%s\n' "$envs" | grep -q "^HOME=$th\$" || return 1
  ! printf '%s\n' "$envs" | grep '^XDG_[A-Z]*_HOME=' | grep -qv "^XDG_[A-Z]*_HOME=$th/"
}
check "the test user's systemd has its own HOME and its own XDG folders" clean_manager_env

# Where the install puts things (ND-270, ND-283, ND-286, ND-294).
prog_dir="$th/.local/share/NetworkDoctor"
prog="$prog_dir/netdoctor"
apps="$th/.local/share/applications"
launcher="$apps/network-doctor.desktop"
uninst="$apps/network-doctor-uninstall.desktop"
results_dir="$th/.config/NetworkDoctor"
units="$th/.config/systemd/user"
autostart="$th/.config/autostart/network-doctor-alerts.desktop"
desk_dir="$th/Desktop"
if d="$(as_user xdg-user-dir DESKTOP 2> /dev/null)" && [ -n "$d" ] && [ "$d" != "$th" ] &&
  case "$d" in "$th"/*) true ;; *) false ;; esac; then
  desk_dir="$d"
fi
deskicon="$desk_dir/network-doctor.desktop"

# desktop_exec FILE [K]: the Exec line of a desktop entry as a shell command line: the string
# escapes undone (\\ to \), %k replaced by the entry's own path, %% to %.
desktop_exec() {
  local raw k
  raw="$(sudo sed -n 's/^Exec=//p' "$1" | head -n 1)"
  raw="$(printf '%s' "$raw" | sed -e 's/\\\\/\\/g')"
  k="$(printf '%q' "${2:-$1}")" # the path quoted for the shell
  raw="${raw//%k/"$k"}"
  printf '%s' "${raw//%%/%}"
}

rows() { sudo sh -c "tail -n +2 '$results_dir/weekly.csv' 2> /dev/null | wc -l"; }
alert_lines() { sudo sh -c "cat '$results_dir/alerts.log' 2> /dev/null | wc -l"; }
state_port() { sudo sed -n 's/.*"port":"\([0-9]*\)".*/\1/p' "$results_dir/open-page.json" 2> /dev/null; }
cron_marks() { as_user crontab -l 2> /dev/null | grep -c '# network-doctor-weekly$'; }
http_code() { curl -s -o /dev/null -m 10 -w '%{http_code}' "$@"; }
no_nd_process() { ! pgrep -u "$tu" -f netdoctor > /dev/null; }
port_refused() { ! curl -s -o /dev/null -m 3 "http://127.0.0.1:$1/api/health"; }
proc_gone() { ! ps -p "$1" > /dev/null; }
the_file_names() { sudo grep -q "\"port\":\"$1\"" "$results_dir/open-page.json"; }
port_changed() { local p; p="$(state_port)"; [ -n "$p" ] && [ "$p" != "$1" ]; }
# These read the state again at every try of waitfor.
has_port() { [ -n "$(state_port)" ]; }
more_alerts() { [ "$(alert_lines)" -gt "$1" ]; }
# True when user systemd answers and lists no unit of this program.
no_nd_units() {
  local list
  list="$(as_user systemctl --user list-unit-files --no-pager)" || return 1
  ! printf '%s\n' "$list" | grep -q network-doctor
}
alerts_active() { [ "$(as_user systemctl --user is-active network-doctor-alerts.service)" = active ]; }

# unit_diag UNIT: when a step with a user unit fails, the evidence goes into the results: the
# unit's text, its status, its journal (read as the user and as root), the user manager's
# environment, and the unit's own ExecStart run by hand as the test user, with its exit status.
unit_diag() {
  local unit="$1" cmd rc
  {
    echo "        --- evidence for $unit"
    echo "        unit text:"
    sudo cat "$units/$unit" 2>&1 | sed 's/^/          /'
    echo "        systemctl --user status $unit:"
    as_user systemctl --user status "$unit" --no-pager -l 2>&1 | sed 's/^/          /'
    echo "        journalctl --user -u $unit (as the test user):"
    as_user journalctl --user -u "$unit" --no-pager -n 30 2>&1 | sed 's/^/          /'
    echo "        journalctl _UID=$tuid _SYSTEMD_USER_UNIT=$unit (as root):"
    sudo journalctl "_UID=$tuid" "_SYSTEMD_USER_UNIT=$unit" --no-pager -n 30 2>&1 | sed 's/^/          /'
    echo "        systemctl --user show-environment:"
    as_user systemctl --user show-environment 2>&1 | sed 's/^/          /'
  } | tee -a "$results" > /dev/null
  # Run by hand in the user manager's own environment (systemd-run), as the unit would run, not
  # in this script's clean one. The alerts helper never ends by itself: it gets 20 seconds and
  # the test switch, and its output is what counts, not its exit status.
  local extra=(-p RuntimeMaxSec=290)
  case "$unit" in *alerts*) extra=(-p RuntimeMaxSec=20 -E NETDOCTOR_TEST_ALERT=1) ;; esac
  cmd="$(sudo sed -n 's/^ExecStart=//p' "$units/$unit" 2> /dev/null | head -n 1)"
  if [ -n "$cmd" ]; then
    as_user timeout 300 systemd-run --user --wait --pipe --collect "${extra[@]}" sh -c "$cmd" \
      < /dev/null > "$out/logs/hand-$unit.log" 2>&1
    rc=$?
    {
      echo "        ExecStart run by hand through systemd-run --user: $cmd (exit $rc):"
      tail -n 15 "$out/logs/hand-$unit.log" | sed 's/^/          /'
    } | tee -a "$results" > /dev/null
  fi
}

# weekly_said LOG: the result line the weekly command printed: "saved", "not-saved" (the speed
# could not be measured, so on purpose no row), or nothing.
weekly_said() {
  if grep -q '^Weekly check saved to ' "$1"; then
    echo saved
  elif grep -q '^Weekly check not saved: the speed was not measured' "$1"; then
    echo not-saved
  fi
}

# weekly_rc_ok RC LOG: the weekly command ends with 0, or, after it said it saved a row, with the
# finding's own code 10 to 14 (as the check command does). Anything else (1, 2, none) fails.
# Through user systemd the unit's SuccessExitStatus makes those codes a finished run, so
# systemctl --user start itself must exit 0.
weekly_rc_ok() {
  [ "$1" = 0 ] && return 0
  case "$1" in 10 | 11 | 12 | 13 | 14) [ "$(weekly_said "$2")" = saved ] ;; *) return 1 ;; esac
}

# weekly_rows_check LOG BEFORE AFTER WHERE: the row count must follow what the program said it
# did: one more row when it said saved, the same count when it said the speed was not measured.
# No result line at all is a failure: the job did not run to its end.
weekly_rows_check() {
  local log="$1" before="$2" after="$3" where="$4" said
  said="$(weekly_said "$log")"
  case "$said" in
    saved)
      check "$where: the program said it saved a row, and weekly.csv has one more ($before to $after) (ND-310)" \
        test "$after" -eq $((before + 1)) || return 1
      ;;
    not-saved)
      note "$where: this machine could not measure the speed, so the program saved no row, as designed"
      check "$where: the program said no row was saved, and weekly.csv has none added ($before to $after) (ND-310)" \
        test "$after" -eq "$before" || return 1
      ;;
    *)
      fail "$where: the weekly command printed neither 'Weekly check saved' nor 'Weekly check not saved' (ND-310)"
      return 1
      ;;
  esac
}

no_install_page() { ! pgrep -u "$tu" -f 'netdoctor install$' > /dev/null; }
newest_open() { pgrep -n -u "$tu" -f 'netdoctor open$'; }

# gio_launch FILE LOG: start a desktop entry the way the desktop does (GLib's launcher, which
# fills in %k and the rest), as the test user, from / as the working folder.
gio_launch() {
  as_user sh -c 'cd / && exec gio launch "$1"' sh "$1" > "$2" 2>&1
}

# exec_absolute FILE: the entry's Exec starts with an absolute path, with no ~ and no ./.
exec_absolute() {
  local e
  e="$(sudo sed -n 's/^Exec=//p' "$1" | head -n 1)"
  case "$e" in /* | \"/*) ;; *) return 1 ;; esac
  ! printf '%s' "$e" | grep -qE '~|(^|[ "])\./'
}

shot() { # shot PLAN.json LOG
  node "$here/shot.mjs" "$1" > "$2" 2>&1
  local rc=$?
  sed 's/^/        /' "$2" | tee -a "$results" > /dev/null
  return $rc
}

say "Network Doctor on Linux, proof run $(date -u +%Y-%m-%dT%H%MZ)"
say "Setup file: $(basename "$tarball") ($(stat -c %s "$tarball") bytes)"
say "Test user home: $th; Desktop folder: $desk_dir"

# The machine must start clean.
check "the test user starts with no program, launcher or job" \
  not ut -e "$prog_dir" -o -e "$launcher" -o -e "$units" -o -e "$autostart"

# The alerts helper raises one test alert at its start when NETDOCTOR_TEST_ALERT=1 is in its
# environment (ND-311). Through user systemd the value is handed to the user manager just before
# the helper starts and taken back after; started directly, it is in the command's environment.
test_words="This is a test alert from Network Doctor."
nd288="Alerts could not be shown on the screen. They are still saved in the alerts file."

# ---------------------------------------------------------------------------------------------
say "== The setup download (ND-278)"
cp "$tarball" /tmp/nd-setup.tar.gz
chmod 0644 /tmp/nd-setup.tar.gz
tar -tvzf /tmp/nd-setup.tar.gz | tee -a "$results"
as_user mkdir -p "$th/Downloads"
as_user tar -C "$th/Downloads" -xzf /tmp/nd-setup.tar.gz
setup_dir="$(sudo find "$th/Downloads" -mindepth 1 -maxdepth 1 -type d | head -n 1)"
setup_entry="$setup_dir/Install Network Doctor.desktop"
check "the download holds the program, runnable" ut -x "$setup_dir/netdoctor"
check "the download holds GUIDE.md" ut -f "$setup_dir/GUIDE.md"
check "the download holds Install Network Doctor.desktop, runnable" ut -x "$setup_entry"
check "the setup launcher has Terminal=false" sudo grep -qx 'Terminal=false' "$setup_entry"
check "the setup launcher passes desktop-file-validate" sudo desktop-file-validate "$setup_entry"

say "== The install page, started as a desktop starts the setup launcher (ND-271, ND-296)"
setup_exec="$(sudo sed -n 's/^Exec=//p' "$setup_entry" | head -n 1)"
say "  setup Exec: $setup_exec"
if printf '%s' "$setup_exec" | grep -qE '(^|[ "=])\./'; then
  fail "the setup launcher finds the program with ./, which breaks when it is not started from its own folder"
  note "FAULT for the Go code: Install Network Doctor.desktop uses ./ to find the program beside it"
elif printf '%s' "$setup_exec" | grep -q '%k'; then
  pass "the setup launcher finds the program beside it from its own place (%k)"
else
  fail "the setup launcher has neither %k nor a way to find the program beside it"
fi
: > "$urls"
gio_launch "$setup_entry" "$out/logs/install-page.log"
check "gio launch of the setup launcher, from /, was accepted" test "$?" -eq 0
if waitfor 30 test -s "$urls"; then
  url="$(head -n 1 "$urls")"
  case "$url" in
    http://127.0.0.1:*) pass "the install page is served on 127.0.0.1 only: ${url%%\?*}" ;;
    *) fail "the install page address is not on 127.0.0.1" ;;
  esac
  cat > /tmp/nd-plan-install.json << EOF
{"url": "$url", "steps": [
  {"size": [1280, 800]},
  {"wait": "document.getElementById('sWelcome') && !document.getElementById('sWelcome').hidden && document.querySelectorAll('#parts input').length === 3", "timeout": 60},
  {"shot": "$out/pictures/linux-install-choices.png"},
  {"size": [390, 844]},
  {"shot": "$out/pictures/linux-install-choices-phone.png"},
  {"fits": "the install choices at phone width"},
  {"size": [1280, 800]},
  {"click": "toOokla"},
  {"wait": "!document.getElementById('sOokla').hidden", "timeout": 10},
  {"shot": "$out/pictures/linux-install-ookla.png"},
  {"click": "toReady"},
  {"wait": "!document.getElementById('sReady').hidden", "timeout": 10},
  {"shot": "$out/pictures/linux-install-ready.png"},
  {"click": "go"},
  {"wait": "!document.getElementById('sEnd').hidden && document.getElementById('sEnd').className.indexOf('done') >= 0", "timeout": 120},
  {"shot": "$out/pictures/linux-install-finish.png"},
  {"size": [390, 844]},
  {"shot": "$out/pictures/linux-install-finish-phone.png"},
  {"fits": "the finish screen at phone width"},
  {"wait": "!document.getElementById('guideOpen').hidden", "timeout": 10},
  {"click": "guideOpen"},
  {"wait": "!document.getElementById('guideText').hidden && /Network Doctor/.test(document.getElementById('guideText').textContent)", "timeout": 10},
  {"shot": "$out/pictures/linux-install-guide-phone.png"},
  {"fits": "the guide shown on the finish screen at phone width"}
]}
EOF
  check "pictures of the install choices and the finish screen at 1280x800 and 390x844, fitting 390 wide; the guide shown by its button" \
    shot /tmp/nd-plan-install.json "$out/logs/shot-install.log"
else
  fail "the setup launcher did not open the install page within 30 seconds"
fi
check "the install page quits by itself after the finish screen" waitfor 30 no_install_page
check "the page install put the program in place" ut -x "$prog"
as_user "$prog" uninstall --silent > "$out/logs/install-page-uninstall.log" 2>&1
check "cleared again before the four choices" not ut -e "$prog_dir"

# ---------------------------------------------------------------------------------------------
# page_proof NAME: the launcher's own command, the port, health, pictures, a second press.
page_proof() {
  local name="$1" pid1 port1 port2
  page2_port=""
  sudo rm -f "$results_dir/open-page.json" # a note left by a stopped run would name a dead port
  say "  launcher Exec: $(sudo sed -n 's/^Exec=//p' "$launcher" | head -n 1)"
  check "the launcher's Exec holds an absolute path, with no ~ and no ./" exec_absolute "$launcher"
  : > "$urls"
  gio_launch "$launcher" "$out/logs/$name-open-1.log"
  check "gio launch of the menu launcher, from /, was accepted" test "$?" -eq 0
  if ! waitfor 30 has_port; then
    fail "open-page.json did not name a port within 30 seconds"
    return
  fi
  port1="$(state_port)"
  pid1="$(newest_open)"
  pass "open-page.json names port $port1 (ND-295)"
  check "/api/health answers 200 (ND-295)" test "$(http_code "http://127.0.0.1:$port1/api/health")" = 200
  check "the launcher asked the browser for http://127.0.0.1:$port1/" waitfor 10 grep -qx "http://127.0.0.1:$port1/" "$urls"
  cat > "/tmp/nd-plan-$name.json" << EOF
{"url": "http://127.0.0.1:$port1/", "steps": [
  {"size": [1280, 800]},
  {"wait": "/^Checked at/.test((document.getElementById('when') || {}).textContent || '')", "timeout": 180},
  {"shot": "$out/pictures/linux-$name-page.png"},
  {"size": [390, 844]},
  {"shot": "$out/pictures/linux-$name-page-phone.png"},
  {"fits": "the page after its check at phone width"}
]}
EOF
  check "pictures of the page after its check at 1280x800 and 390x844, fitting 390 wide (ND-296)" \
    shot "/tmp/nd-plan-$name.json" "$out/logs/shot-$name.log"

  # The second press retires the first page (ND-297).
  gio_launch "$launcher" "$out/logs/$name-open-2.log"
  if waitfor 30 port_changed "$port1"; then
    port2="$(state_port)"
    page2_port="$port2"
    pass "the second press wrote a new port, $port2, into open-page.json (ND-297)"
    check "the old page's heartbeat gets 410 from the first server (ND-297)" \
      test "$(http_code -H 'X-NetDoctor: 1' "http://127.0.0.1:$port1/api/alive")" = 410
    check "the first server is gone within 90 seconds (ND-297)" waitfor 90 port_refused "$port1"
    check "the first launcher run has ended" waitfor 30 proc_gone "${pid1:-0}"
    check "open-page.json still names the second port" the_file_names "$port2"
    check "the second page answers /api/health with 200" test "$(http_code "http://127.0.0.1:$port2/api/health")" = 200
  else
    fail "the second press did not write a new port within 30 seconds (ND-297)"
  fi
  # The second page server is left running: uninstall must stop it (ND-312).
}

# removal_start NAME: open the running page in headless Chrome and wait for its check; once ready,
# the caller runs uninstall and the browser waits for the removed notice and takes its picture.
removal_start() {
  local name="$1"
  removal_pid=""
  [ -n "$page2_port" ] || return 0
  rm -f /tmp/nd-removal-ready
  cat > "/tmp/nd-plan-removed-$name.json" << EOF
{"url": "http://127.0.0.1:$page2_port/", "steps": [
  {"size": [1280, 800]},
  {"wait": "/^Checked at/.test((document.getElementById('when') || {}).textContent || '')", "timeout": 180},
  {"touch": "/tmp/nd-removal-ready"},
  {"wait": "document.body.dataset.retired === '1' && /Network Doctor was removed from this computer/.test(document.body.innerText)", "timeout": 120},
  {"shot": "$out/pictures/linux-removed.png"},
  {"size": [390, 844]},
  {"shot": "$out/pictures/linux-removed-phone.png"},
  {"fits": "the removed notice at phone width"}
]}
EOF
  node "$here/shot.mjs" "/tmp/nd-plan-removed-$name.json" > "$out/logs/shot-removed.log" 2>&1 &
  removal_pid=$!
  check "the page is open in headless Chrome before uninstall" waitfor 200 test -f /tmp/nd-removal-ready
}

# removal_finish: after uninstall, the picture of "Network Doctor was removed from this computer."
removal_finish() {
  [ -n "$removal_pid" ] || return 0
  local rc
  wait "$removal_pid"
  rc=$?
  sed 's/^/        /' "$out/logs/shot-removed.log" | tee -a "$results" > /dev/null
  check "the open page says Network Doctor was removed; pictures at 1280x800 and 390x844" test "$rc" -eq 0
}

# after_uninstall_pages: uninstall should have stopped every page server of the program.
after_uninstall_pages() {
  if pgrep -u "$tu" -f 'netdoctor open$' > /dev/null; then
    note "a page server was still running after uninstall; the test stops it (uninstall did not)"
    sudo pkill -INT -u "$tu" -f 'netdoctor open$'
    waitfor 15 not pgrep -u "$tu" -f 'netdoctor open$' || sudo pkill -KILL -u "$tu" -f 'netdoctor open$'
  fi
}

# weekly_systemd: run the weekly job once through user systemd (ND-310).
weekly_systemd() {
  local before after since rc log="$out/logs/$1-weekly-journal.log" ok=1
  before="$(rows)"
  since="$(date '+%Y-%m-%d %H:%M:%S')"
  check "just before the start: the test user's systemd still has its own HOME and XDG folders" clean_manager_env
  sleep 1 # the journal's --since has whole seconds
  as_user timeout 300 systemctl --user start network-doctor-weekly.service
  rc=$?
  after="$(rows)"
  weekly_journal() {
    sudo journalctl "_UID=$tuid" "_SYSTEMD_USER_UNIT=network-doctor-weekly.service" --since "$since" \
      -o cat --no-pager > "$log" 2>&1
    [ -n "$(weekly_said "$log")" ]
  }
  waitfor 15 weekly_journal
  sed 's/^/        /' "$log" | tail -n 5 | tee -a "$results" > /dev/null
  if [ "$rc" -eq 0 ]; then
    pass "systemctl --user start network-doctor-weekly.service ran the job to its end, exit 0 (ND-310)"
  else
    fail "systemctl --user start network-doctor-weekly.service exited $rc (ND-310)"
    ok=0
  fi
  if [ -n "$(weekly_said "$log")" ]; then
    pass "the job's own result line is in its journal"
  else
    fail "the job's journal has no result line from the weekly command"
    ok=0
  fi
  weekly_rows_check "$log" "$before" "$after" "the weekly job run by user systemd" || ok=0
  [ "$ok" -eq 1 ] || unit_diag network-doctor-weekly.service
}

# alert_check NAME LOG BEFORE MODE: wait for one more alert line, then read the run log and the
# screen stand-in's record (ND-311, ND-287, ND-288). MODE "works": the stand-in notify-send was
# first on the helper's PATH and must have been called once with the test alert, and the plain
# line must be absent. MODE "missing": no notify-send on the helper's PATH, so the plain line of
# ND-288 must be there exactly once, and the stand-in never called.
alert_check() {
  local name="$1" log="$2" before="$3" mode="$4" calls said
  if waitfor 60 more_alerts "$before"; then
    pass "the alerts helper wrote one more line to alerts.log (ND-311)"
    sudo tail -n 3 "$results_dir/alerts.log" | sed 's/^/        /' | tee -a "$results" > /dev/null
  else
    fail "no new line in alerts.log within 1 minute (ND-311)"
  fi
  check "the new line is the test alert, kind test, with its words" sudo sh -c \
    "tail -n +$((before + 1)) '$results_dir/alerts.log' | grep -F '	test	' | grep -qF '$test_words'"
  sed 's/^/        /' "$log" | tail -n 8 | tee -a "$results" > /dev/null
  check "the run log shows the test alert" grep -qF "$test_words" "$log"
  said="$(grep -cF "$nd288" "$log")"
  calls="$(grep -cF "$test_words" "$screen_calls" 2> /dev/null)"
  sed 's/^/        screen call: /' "$screen_calls" | tee -a "$results" > /dev/null
  if [ "$mode" = works ]; then
    check "notify-send was called exactly once with the test alert (calls: $calls)" test "${calls:-0}" -eq 1
    check "the stand-in got the title Network Doctor as its own argument" grep -q '^-- Network Doctor ' "$screen_calls"
    check "the run log has no plain line of ND-288, since the screen alert worked" test "$said" -eq 0
  else
    check "with no notify-send, the run log says the plain line of ND-288 exactly once (said: $said)" \
      test "$said" -eq 1
    check "no screen alert was made (stand-in calls: ${calls:-0})" test "${calls:-0}" -eq 0
  fi
}

# screen_works and screen_missing set the PATH the helper gets. The user manager's own PATH is
# saved first and put back after the helper has started.
stub_path="$screen_stub:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
plain_path="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
hidden_notify=""
# hide_real_notify: a machine notify-send would make "missing" untrue, so it is moved aside.
hide_real_notify() {
  local p
  p="$(PATH="$plain_path" command -v notify-send 2> /dev/null)"
  if [ -n "$p" ]; then
    sudo mv "$p" "$p.nd-hidden" && hidden_notify="$p"
    note "this machine's own notify-send was moved aside for the missing case: $p"
  fi
}
restore_real_notify() {
  [ -n "$hidden_notify" ] && sudo mv "$hidden_notify.nd-hidden" "$hidden_notify"
  hidden_notify=""
}

# run_choice NAME PARTS SCHEDULED PROACTIVE REMOVAL, where REMOVAL is keep (--keep-results, the
# results stay), plain (no switch: everything goes), planted (no switch, with a planted undo note:
# exit 1 and only the notes stay) or old (the old --delete-results: everything goes).
run_choice() {
  local name="$1" parts="$2" sched="$3" pro="$4" removal="$5" rc line before log
  say "== Choice: $name (parts: ${parts:-none})"
  as_user "$setup_dir/netdoctor" install --silent --parts="$parts" > "$out/logs/$name-install.log" 2>&1
  rc=$?
  sed 's/^/        /' "$out/logs/$name-install.log" | tee -a "$results" > /dev/null
  check "install --silent --parts=$parts exits 0 (ND-293)" test "$rc" -eq 0

  # Files and icons (ND-270, ND-275, ND-309).
  check "program in ~/.local/share/NetworkDoctor, runnable" ut -x "$prog"
  check "GUIDE.md beside the program" ut -f "$prog_dir/GUIDE.md"
  check "choices.json says scheduled $sched, proactive $pro" sudo grep -qx \
    "{\"core\":true,\"scheduled\":$sched,\"proactive\":$pro,\"ookla\":false}" "$prog_dir/choices.json"
  check "launcher network-doctor.desktop in the applications menu" ut -f "$launcher"
  check "launcher says Name=Network Doctor" sudo grep -qx 'Name=Network Doctor' "$launcher"
  check "launcher says Terminal=false" sudo grep -qx 'Terminal=false' "$launcher"
  check "launcher Exec ends in ' open'" sudo grep -q '^Exec=.* open$' "$launcher"
  check "launcher passes desktop-file-validate" sudo desktop-file-validate "$launcher"
  check "Uninstall Network Doctor in the applications menu" ut -f "$uninst"
  check "the uninstall entry passes desktop-file-validate" sudo desktop-file-validate "$uninst"
  check "the Desktop copy exists and is runnable" ut -x "$deskicon"
  check "the Desktop copy is the same as the launcher" sudo cmp -s "$launcher" "$deskicon"
  # Every file the install made, for the record (ND-309).
  sudo find "$prog_dir" "$apps" "$desk_dir" "$th/.config/systemd" "$th/.config/autostart" \
    -mindepth 1 2> /dev/null | sed 's/^/        /' | tee -a "$results" > /dev/null

  # The weekly job (ND-283).
  if [ "$sched" = true ]; then
    check "weekly service and timer in ~/.config/systemd/user" \
      ut -f "$units/network-doctor-weekly.service" -a -f "$units/network-doctor-weekly.timer"
    check "timer says OnCalendar=Mon *-*-* 09:00:00" \
      sudo grep -qx 'OnCalendar=Mon \*-\*-\* 09:00:00' "$units/network-doctor-weekly.timer"
    check "timer says Persistent=true" sudo grep -qx 'Persistent=true' "$units/network-doctor-weekly.timer"
    check "the weekly service counts the finding codes as a finished run (SuccessExitStatus=10 11 12 13 14)" \
      sudo grep -qx 'SuccessExitStatus=10 11 12 13 14' "$units/network-doctor-weekly.service"
    check "the timer is enabled" test "$(as_user systemctl --user is-enabled network-doctor-weekly.timer)" = enabled
    as_user systemctl --user list-timers --all --no-pager 2>&1 | sed 's/^/        /' | tee -a "$results" > /dev/null
  else
    check "no weekly timer" not ut -e "$units/network-doctor-weekly.timer"
  fi
  check "no crontab line, since user systemd is there" test "$(cron_marks)" -eq 0

  # The alerts job (ND-286).
  if [ "$pro" = true ]; then
    check "alerts autostart entry in ~/.config/autostart" ut -f "$autostart"
    check "the autostart entry starts the user service" \
      sudo grep -q '^Exec=.*systemctl --user start network-doctor-alerts.service' "$autostart"
    check "alerts service with Restart=on-failure and RestartSec=60" sudo sh -c \
      "grep -qx 'Restart=on-failure' '$units/network-doctor-alerts.service' && grep -qx 'RestartSec=60' '$units/network-doctor-alerts.service'"
  else
    check "no alerts autostart entry or service" \
      not ut -e "$autostart" -o -e "$units/network-doctor-alerts.service"
  fi

  page_proof "$name"

  if [ "$sched" = true ]; then
    weekly_systemd "$name"
  fi

  if [ "$pro" = true ]; then
    before="$(alert_lines)"
    since="$(date '+%Y-%m-%d %H:%M:%S')"
    line="$(desktop_exec "$autostart")"
    say "  autostart line: $line"
    # proactive: a working screen alert (stand-in first on PATH); both: no notify-send at all.
    mode=works
    hpath="$stub_path"
    if [ "$name" = both ]; then
      mode=missing
      hpath="$plain_path"
      hide_real_notify
    fi
    : > "$screen_calls"
    orig_path="$(as_user systemctl --user show-environment | grep '^PATH=')"
    check "just before the start: the test user's systemd still has its own HOME and XDG folders" clean_manager_env
    as_user systemctl --user set-environment NETDOCTOR_TEST_ALERT=1 "PATH=$hpath"
    as_user sh -c "$line" > "$out/logs/$name-autostart.log" 2>&1
    check "the alerts service runs after the autostart line" waitfor 20 alerts_active
    as_user systemctl --user unset-environment NETDOCTOR_TEST_ALERT
    if [ -n "$orig_path" ]; then
      as_user systemctl --user set-environment "$orig_path"
    else
      as_user systemctl --user unset-environment PATH
    fi
    say "  screen case: $mode (helper PATH $hpath)"
    log="$out/logs/$name-alerts-journal.log"
    waitfor 60 more_alerts "$before"
    journal_read() {
      sudo journalctl "_UID=$tuid" "_SYSTEMD_USER_UNIT=network-doctor-alerts.service" --since "$since" -o cat --no-pager > "$log" 2>&1
    }
    journal_has() { journal_read; grep -qF "$nd288" "$log"; }
    if [ "$mode" = works ]; then
      waitfor 20 grep -qF "$test_words" "$screen_calls"
    else
      waitfor 40 journal_has
    fi
    sleep 2
    journal_read
    f0=$fails
    alert_check "$name" "$log" "$before" "$mode"
    [ "$fails" -gt "$f0" ] && unit_diag network-doctor-alerts.service
    restore_real_notify
  fi

  # Uninstall (ND-290, ND-291, ND-312). On the first choice the second page is open in headless
  # Chrome meanwhile, for the picture of the removed notice.
  # On the other choices the page left running from the second press is watched instead: its
  # heartbeat, sent while uninstall runs, must get 410 with the body "removed".
  removal_pid=""
  old_port="$page2_port"
  if [ "$name" = neither ]; then
    removal_start "$name"
  fi
  un_args=(uninstall --silent)
  [ "$removal" = keep ] && un_args+=(--keep-results)
  [ "$removal" = old ] && un_args+=(--delete-results)
  want_rc=0
  if [ "$removal" = planted ]; then
    # ND-316: an undo notes file that cannot be read, so the undo changes nothing and counts it as
    # not put back: the removal exits 1 and only the undo notes stay. (A note for a setting changed
    # by hand since is settled at removal and dropped, so it would not prove the kept notes.)
    printf '%s' 'not notes' > /tmp/nd-planted-note.json
    as_user mkdir -p "$results_dir"
    sudo install -o "$tu" -m 644 /tmp/nd-planted-note.json "$results_dir/fix-saved.json"
    proxy_before="$(as_user gsettings get org.gnome.system.proxy mode 2> /dev/null || echo none-there)"
    want_rc=1
  fi
  say "  ${un_args[*]}"
  rm -f /tmp/nd-un-rc /tmp/nd-410-body
  (as_user "$prog" "${un_args[@]}" > "$out/logs/$name-uninstall.log" 2>&1; echo $? > /tmp/nd-un-rc) &
  un_pid=$!
  beat=""
  if [ -z "$removal_pid" ] && [ -n "$old_port" ]; then
    while ps -p "$un_pid" > /dev/null; do
      beat="$(curl -s -m 2 -o /tmp/nd-410-body -w '%{http_code}' -H 'X-NetDoctor: 1' "http://127.0.0.1:$old_port/api/alive")"
      [ "$beat" = 410 ] && break
      sleep 0.2
    done
  fi
  wait "$un_pid"
  rc="$(cat /tmp/nd-un-rc 2> /dev/null || echo 1)"
  check "uninstall exits $want_rc (got $rc)" test "$rc" -eq "$want_rc"
  if [ -n "$removal_pid" ]; then
    removal_finish
  elif [ -n "$old_port" ]; then
    check "the open page's heartbeat during uninstall got 410 (got: ${beat:-none})" test "$beat" = 410
    check "and the body said removed" grep -qx 'removed' /tmp/nd-410-body
  fi
  if [ -n "$old_port" ]; then
    check "the open page's port $old_port is closed after uninstall" waitfor 10 port_refused "$old_port"
  fi
  after_uninstall_pages
  check "no netdoctor process left (pgrep)" waitfor 20 no_nd_process
  check "program folder gone" not ut -e "$prog_dir"
  check "launcher and uninstall entry gone" not ut -e "$launcher" -o -e "$uninst"
  check "Desktop copy gone" not ut -e "$deskicon"
  check "weekly service and timer gone" \
    not ut -e "$units/network-doctor-weekly.timer" -o -e "$units/network-doctor-weekly.service"
  check "alerts service and autostart entry gone" not ut -e "$autostart" -o -e "$units/network-doctor-alerts.service"
  check "user systemd knows no network-doctor unit" no_nd_units
  check "no marked crontab line" test "$(cron_marks)" -eq 0
  if [ "$removal" = keep ]; then
    check "results folder kept with --keep-results" ut -d "$results_dir"
  elif [ "$removal" = planted ]; then
    sed 's/^/        /' "$out/logs/$name-uninstall.log" | tee -a "$results" > /dev/null
    check "the removal said the change could not be put back" grep -q 'could not put back' "$out/logs/$name-uninstall.log"
    check "the proxy setting was not changed" test \
      "$(as_user gsettings get org.gnome.system.proxy mode 2> /dev/null || echo none-there)" = "$proxy_before"
    check "only the undo notes are left in the results folder" sudo sh -c \
      "test -f '$results_dir/fix-saved.json' && [ -z \"\$(ls -A '$results_dir' | grep -v -x -e fix-saved.json -e fix-log.txt -e fix-enabled-here.txt)\" ]"
    sudo rm -rf "$results_dir" # the planted note is the test's own; the next choice starts clean
  else
    check "results folder and settings deleted (${un_args[*]})" not ut -e "$results_dir"
  fi
}

run_choice neither "" false false keep
run_choice scheduled "scheduled" true false planted
run_choice proactive "proactive" false true keep
run_choice both "scheduled,proactive" true true old

# ---------------------------------------------------------------------------------------------
say "== No user systemd: the crontab line and the direct autostart (ND-283, ND-286)"
upath="$nosd:$base_path"
check "systemctl is not reachable for this pass" not as_user systemctl --user show-environment
as_user "$setup_dir/netdoctor" install --silent --parts=scheduled,proactive > "$out/logs/cron-install.log" 2>&1
rc=$?
sed 's/^/        /' "$out/logs/cron-install.log" | tee -a "$results" > /dev/null
check "install --silent --parts=scheduled,proactive exits 0" test "$rc" -eq 0
check "no timer or alerts service files" not ut -e "$units/network-doctor-weekly.timer" -o -e "$units/network-doctor-alerts.service"
as_user crontab -l 2>&1 | sed 's/^/        /' | tee -a "$results" > /dev/null
check "one marked crontab line, 0 9 * * 1, running weekly" test "$(as_user crontab -l 2> /dev/null |
  grep -c '^0 9 \* \* 1 .* weekly # network-doctor-weekly$')" -eq 1
check "the autostart entry starts the helper directly" sudo sh -c \
  "grep -q '^Exec=.* watch$' '$autostart' && ! grep -q systemctl '$autostart'"

before="$(rows)"
cron_cmd="$(as_user crontab -l 2> /dev/null | grep '# network-doctor-weekly$' | head -n 1 |
  sed -E 's/^([^ ]+ +){5}//; s/ *# network-doctor-weekly$//')"
say "  crontab command: $cron_cmd"
as_user timeout 300 sh -c "$cron_cmd" > "$out/logs/cron-weekly.log" 2>&1
rc=$?
sed 's/^/        /' "$out/logs/cron-weekly.log" | tail -n 5 | tee -a "$results" > /dev/null
check "the crontab line's own command ended well: 0, or the finding's code 10 to 14 after a saved row (got $rc) (ND-310)" \
  weekly_rc_ok "$rc" "$out/logs/cron-weekly.log"
weekly_rows_check "$out/logs/cron-weekly.log" "$before" "$(rows)" "the crontab line's own command"

before="$(alert_lines)"
line="$(desktop_exec "$autostart")"
say "  autostart line: $line"
# The screen case here is "works": the notify-send stand-in is first on the helper's PATH.
: > "$screen_calls"
as_user env NETDOCTOR_TEST_ALERT=1 PATH="$screen_stub:$upath" sh -c "$line" > "$out/logs/cron-alerts.log" 2>&1 &
waitfor 60 more_alerts "$before"
waitfor 20 grep -qF "$test_words" "$screen_calls"
sleep 2
alert_check cron "$out/logs/cron-alerts.log" "$before" works

as_user "$prog" uninstall --silent > "$out/logs/cron-uninstall.log" 2>&1
check "uninstall --silent exits 0" test "$?" -eq 0
check "no netdoctor process left (pgrep): the helper was stopped" waitfor 20 no_nd_process
check "the marked crontab line is gone" test "$(cron_marks)" -eq 0
check "autostart entry, program, launchers and Desktop copy gone" \
  not ut -e "$autostart" -o -e "$prog_dir" -o -e "$launcher" -o -e "$uninst" -o -e "$deskicon"
check "results folder and settings deleted with no switch" not ut -e "$results_dir"
upath="$base_path"

# ---------------------------------------------------------------------------------------------
say "== Done: $fails failed check(s)."
[ "$fails" -eq 0 ]
