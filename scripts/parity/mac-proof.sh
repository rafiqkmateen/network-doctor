#!/usr/bin/env bash
# Purpose: prove the Mac setup on GitHub's clean Mac machine (ND-269, ND-271, ND-276, ND-277,
# ND-282, ND-285, ND-290, ND-291, ND-295 to ND-297, ND-309 to ND-312): install each of the four
# choices with the real program, check every file, icon and LaunchAgent, open the page with the
# app's own start file, take pictures, press it twice, run the weekly job and the alerts helper
# through launchd, then uninstall: keeping the results with --keep-results, and with no switch and
# with the old --delete-results, which both delete everything; before the plain one an undo note
# for a fix never applied is planted, to prove the undo step runs first and keeps the notes (ND-316).
# Owner: Rafiq. Version 0.5.0 (0.5.0, 2026-11-23: the planted undo note; 0.4.0, 2026-11-23: a plain silent uninstall deletes the results,
# --keep-results keeps them, the old --delete-results is checked to still work; 2026-11-20 later: the weekly check follows the program's own
# result line and launchd's exit code, evidence printed when a job step fails; 0.2.0,
# 2026-11-20: both apps started with open from another folder, the
# removed notice pictured, the test alert; 0.1.0: first build).
#
# Use: bash scripts/parity/mac-proof.sh SETUP.zip OUTDIR
# Run it only on a throwaway machine. It uses the machine's own user and home folder, because
# launchd starts the jobs with that home: GitHub's Mac is new for every run, and the script first
# checks that nothing of Network Doctor is there.
set -uo pipefail # not -e: every check runs and is counted

zipfile="$(cd "$(dirname "$1")" && pwd)/$(basename "$1")"
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
check() {
  local what="$1"
  shift
  if "$@" > /dev/null 2>&1; then pass "$what"; else fail "$what"; return 1; fi
}
not() { ! "$@"; }
waitfor() {
  local end=$((SECONDS + $1))
  shift
  while [ "$SECONDS" -lt "$end" ]; do
    if "$@" > /dev/null 2>&1; then return 0; fi
    sleep 1
  done
  return 1
}

uid="$(id -u)"
dom="gui/$uid"
app="$HOME/Applications/Network Doctor.app"
uapp="$HOME/Applications/Uninstall Network Doctor.app"
prog="$app/Contents/MacOS/netdoctor"
launcher="$app/Contents/MacOS/Network Doctor"
alias_path="$HOME/Desktop/Network Doctor"
agents="$HOME/Library/LaunchAgents"
wplist="$agents/com.networkdoctor.weekly.plist"
aplist="$agents/com.networkdoctor.alerts.plist"
results_dir="$HOME/Library/Application Support/NetworkDoctor"

# A browser stand-in, found before the system's own open command only by Network Doctor's runs:
# it records the address instead of opening a browser.
tools=/tmp/nd-tools
mkdir -p "$tools"
urls="$tools/urls.txt"
cat > "$tools/open" << EOF
#!/bin/sh
# Records the address it was asked to open, instead of opening a browser.
printf '%s\n' "\$1" >> "$urls"
EOF
chmod 0755 "$tools/open"
# An osascript stand-in for the alerts helper: it records its arguments and succeeds.
screen_stub=/tmp/nd-screen
screen_calls="$screen_stub/calls.txt"
mkdir -p "$screen_stub"
cat > "$screen_stub/osascript" << EOF
#!/bin/sh
# Records the screen alert it was asked to show, as one line, and reports success.
printf '%s\n' "\$*" >> "$screen_calls"
EOF
chmod 0755 "$screen_stub/osascript"
: > "$screen_calls"
: > "$urls"
with_recorder() { env PATH="$tools:$PATH" "$@"; }

rows() { tail -n +2 "$results_dir/weekly.csv" 2> /dev/null | wc -l | tr -d ' '; }
alert_lines() { cat "$results_dir/alerts.log" 2> /dev/null | wc -l | tr -d ' '; }
state_port() { sed -n 's/.*"port":"\([0-9]*\)".*/\1/p' "$results_dir/open-page.json" 2> /dev/null; }
http_code() { curl -s -o /dev/null -m 10 -w '%{http_code}' "$@"; }
no_nd_process() { ! pgrep -f 'MacOS/netdoctor' > /dev/null; }
port_refused() { ! curl -s -o /dev/null -m 3 "http://127.0.0.1:$1/api/health"; }
proc_gone() { ! ps -p "$1" > /dev/null; }
the_file_names() { grep -q "\"port\":\"$1\"" "$results_dir/open-page.json"; }
port_changed() { local p; p="$(state_port)"; [ -n "$p" ] && [ "$p" != "$1" ]; }
has_port() { [ -n "$(state_port)" ]; }
more_rows() { [ "$(rows)" -gt "$1" ]; }
more_alerts() { [ "$(alert_lines)" -gt "$1" ]; }
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
weekly_rc_ok() {
  [ "$1" = 0 ] && return 0
  case "$1" in 10 | 11 | 12 | 13 | 14) [ "$(weekly_said "$2")" = saved ] ;; *) return 1 ;; esac
}
# weekly_rows_check LOG BEFORE AFTER WHERE: one more row when the program said saved, the same
# count when it said the speed was not measured; no result line at all is a failure.
weekly_rows_check() {
  local log="$1" before="$2" after="$3" where="$4"
  case "$(weekly_said "$log")" in
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
loaded() { launchctl print "$dom/$1"; }
plist_is() { [ "$(/usr/libexec/PlistBuddy -c "Print :$2" "$1" 2> /dev/null)" = "$3" ]; }
gone_path() { [ ! -e "$1" ] && [ ! -L "$1" ]; }
setup_running() { pgrep -f 'Network Doctor Setup.app/Contents/MacOS/netdoctor install$'; }
# listen_port PID: the TCP port the process listens on.
listen_port() { lsof -nP -a -p "$1" -iTCP -sTCP:LISTEN 2> /dev/null | awk 'NR>1{sub(/.*:/,"",$9); print $9; exit}'; }
# answers PORT: any HTTP answer at all (the setup page refuses a request without its token).
answers() { [ "$(http_code "http://127.0.0.1:$1/")" != 000 ]; }
# quit_browser: the page went to the machine's own browser, which is closed again.
quit_browser() { pkill -x Safari > /dev/null 2>&1 || true; }

shot() {
  node "$here/shot.mjs" "$1" > "$2" 2>&1
  local rc=$?
  sed 's/^/        /' "$2" | tee -a "$results" > /dev/null
  return $rc
}

say "Network Doctor on a Mac, proof run $(date -u +%Y-%m-%dT%H%MZ), macOS $(sw_vers -productVersion), $(uname -m)"
say "Setup file: $(basename "$zipfile") ($(wc -c < "$zipfile" | tr -d ' ') bytes)"
check "the machine starts with no Network Doctor app, agent or results" \
  not test -e "$app" -o -e "$wplist" -o -e "$aplist" -o -e "$results_dir"
check "this user's launchd session answers (launchctl print $dom)" launchctl print "$dom"

# The alerts helper raises one test alert at its start when NETDOCTOR_TEST_ALERT=1 is in its
# environment (ND-311). launchd hands it to the agents it starts after `launchctl setenv`; the
# value is taken back with `launchctl unsetenv` once the helper is running.
test_words="This is a test alert from Network Doctor."
nd288="Alerts could not be shown on the screen. They are still saved in the alerts file."

# ---------------------------------------------------------------------------------------------
say "== The setup download (ND-276, ND-277)"
unzip -l "$zipfile" | tee -a "$results"
dl="$HOME/Downloads/nd-setup"
rm -rf "$dl"
mkdir -p "$dl"
ditto -x -k "$zipfile" "$dl"
setup_app="$dl/Network Doctor Setup.app"
check "the zip holds Network Doctor Setup.app" test -d "$setup_app"
check "the zip holds How to open Network Doctor Setup.html" test -f "$dl/How to open Network Doctor Setup.html"
archs="$(lipo -archs "$setup_app/Contents/MacOS/netdoctor" 2> /dev/null)"
say "  lipo -archs: $archs"
check "the program holds both arm64 and x86_64" sh -c "echo '$archs' | grep -qw arm64 && echo '$archs' | grep -qw x86_64"
check "the guide is in the app's Resources folder" test -f "$setup_app/Contents/Resources/GUIDE.md"
check "the setup app's signature checks out (codesign --verify --deep --strict)" \
  codesign --verify --deep --strict "$setup_app"
check "the setup app's Info.plist passes plutil -lint" plutil -lint "$setup_app/Contents/Info.plist"
check "the setup app has LSUIElement true" plist_is "$setup_app/Contents/Info.plist" LSUIElement true

say "== The install page, started by the setup app's own start file (ND-271, ND-296)"
: > "$urls"
with_recorder "$setup_app/Contents/MacOS/Network Doctor Setup" > "$out/logs/install-page.log" 2>&1 &
page_pid=$!
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
  {"shot": "$out/pictures/mac-install-choices.png"},
  {"size": [390, 844]},
  {"shot": "$out/pictures/mac-install-choices-phone.png"},
  {"size": [1280, 800]},
  {"click": "toOokla"},
  {"wait": "!document.getElementById('sOokla').hidden", "timeout": 10},
  {"shot": "$out/pictures/mac-install-ookla.png"},
  {"click": "toReady"},
  {"wait": "!document.getElementById('sReady').hidden", "timeout": 10},
  {"shot": "$out/pictures/mac-install-ready.png"},
  {"click": "go"},
  {"wait": "!document.getElementById('sEnd').hidden && document.getElementById('sEnd').className.indexOf('done') >= 0", "timeout": 120},
  {"shot": "$out/pictures/mac-install-finish.png"},
  {"size": [390, 844]},
  {"shot": "$out/pictures/mac-install-finish-phone.png"}
]}
EOF
  check "pictures of the install choices and the finish screen at 1280x800 and 390x844" \
    shot /tmp/nd-plan-install.json "$out/logs/shot-install.log"
else
  fail "the setup app did not open the install page within 30 seconds"
fi
check "the install page quits by itself after the finish screen" waitfor 60 proc_gone "$page_pid"
check "the page install made the app" test -x "$prog"
"$prog" uninstall --silent > "$out/logs/install-page-uninstall.log" 2>&1
check "cleared again before the four choices" not test -e "$app"

say "== The setup app started as Finder does, with open, from another folder (ND-271)"
(cd / && open "$setup_app") > "$out/logs/open-setup.log" 2>&1
check "open \"Network Doctor Setup.app\", from /, was accepted" test "$?" -eq 0
if waitfor 30 setup_running; then
  spid="$(setup_running | head -n 1)"
  sport=""
  for _ in 1 2 3 4 5 6 7 8 9 10; do
    sport="$(listen_port "$spid")"
    [ -n "$sport" ] && break
    sleep 1
  done
  if [ -n "$sport" ] && answers "$sport"; then
    pass "the setup app started by open serves its page on 127.0.0.1:$sport"
  else
    fail "the setup app started by open runs but its page does not answer"
  fi
else
  fail "the setup app started by open did not start the setup program within 30 seconds"
fi
# Nothing is installed: the page was never pressed. The setup program is stopped again.
pkill -INT -f 'Network Doctor Setup.app/Contents/MacOS/netdoctor install$'
waitfor 15 not setup_running || pkill -KILL -f 'Network Doctor Setup.app/Contents/MacOS/netdoctor install$'
quit_browser
check "nothing was installed by the setup app started by open" not test -e "$app"

# ---------------------------------------------------------------------------------------------
page_proof() {
  local name="$1" pid1 pid2 port1 port2
  page2_port=""
  rm -f "$results_dir/open-page.json" # a note left by a stopped run would name a dead port
  : > "$urls"
  with_recorder "$launcher" > "$out/logs/$name-open-1.log" 2>&1 &
  pid1=$!
  if ! waitfor 30 has_port; then
    fail "open-page.json did not name a port within 30 seconds"
    return
  fi
  port1="$(state_port)"
  pass "open-page.json names port $port1 (ND-295)"
  check "/api/health answers 200 (ND-295)" test "$(http_code "http://127.0.0.1:$port1/api/health")" = 200
  check "the app asked the browser for http://127.0.0.1:$port1/" waitfor 10 grep -qx "http://127.0.0.1:$port1/" "$urls"
  cat > "/tmp/nd-plan-$name.json" << EOF
{"url": "http://127.0.0.1:$port1/", "steps": [
  {"size": [1280, 800]},
  {"wait": "/^Checked at/.test((document.getElementById('when') || {}).textContent || '')", "timeout": 180},
  {"shot": "$out/pictures/mac-$name-page.png"},
  {"size": [390, 844]},
  {"shot": "$out/pictures/mac-$name-page-phone.png"}
]}
EOF
  check "pictures of the page after its check at 1280x800 and 390x844 (ND-296)" \
    shot "/tmp/nd-plan-$name.json" "$out/logs/shot-$name.log"

  with_recorder "$launcher" > "$out/logs/$name-open-2.log" 2>&1 &
  pid2=$!
  if waitfor 30 port_changed "$port1"; then
    port2="$(state_port)"
    page2_port="$port2"
    pass "the second press wrote a new port, $port2, into open-page.json (ND-297)"
    check "the old page's heartbeat gets 410 from the first server (ND-297)" \
      test "$(http_code -H 'X-NetDoctor: 1' "http://127.0.0.1:$port1/api/alive")" = 410
    check "the first server is gone within 90 seconds (ND-297)" waitfor 90 port_refused "$port1"
    check "the first app run has ended" waitfor 30 proc_gone "$pid1"
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
  {"shot": "$out/pictures/mac-removed.png"},
  {"size": [390, 844]},
  {"shot": "$out/pictures/mac-removed-phone.png"}
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
  if pgrep -f 'MacOS/netdoctor open$' > /dev/null; then
    note "a page server was still running after uninstall; the test stops it (uninstall did not)"
    pkill -INT -f 'MacOS/netdoctor open$'
    waitfor 15 not pgrep -f 'MacOS/netdoctor open$' || pkill -KILL -f 'MacOS/netdoctor open$'
  fi
}

# open_app_proof: start the installed app as Finder does, with `open`, from another folder. The
# page then goes to the machine's own browser, so the proof reads open-page.json and asks health.
open_app_proof() {
  local port
  rm -f "$results_dir/open-page.json"
  (cd / && open "$app") > "$out/logs/open-app.log" 2>&1
  check "open \"~/Applications/Network Doctor.app\", from /, was accepted" test "$?" -eq 0
  if waitfor 30 has_port; then
    port="$(state_port)"
    pass "the app started by open serves the page: open-page.json names port $port"
    check "/api/health answers 200 for the app started by open" \
      test "$(http_code "http://127.0.0.1:$port/api/health")" = 200
  else
    fail "the app started by open did not start the page within 30 seconds"
  fi
  pkill -INT -f 'MacOS/netdoctor open$'
  waitfor 15 not pgrep -f 'MacOS/netdoctor open$' || pkill -KILL -f 'MacOS/netdoctor open$'
  quit_browser
}

# run_choice NAME PARTS SCHEDULED PROACTIVE REMOVAL, where REMOVAL is keep (--keep-results, the
# results stay), plain (no switch: everything goes), planted (no switch, with a planted undo note:
# exit 1 and only the notes stay) or old (the old --delete-results: everything goes).
run_choice() {
  local name="$1" parts="$2" sched="$3" pro="$4" removal="$5" rc before
  say "== Choice: $name (parts: ${parts:-none})"
  "$setup_app/Contents/MacOS/netdoctor" install --silent --parts="$parts" > "$out/logs/$name-install.log" 2>&1
  rc=$?
  sed 's/^/        /' "$out/logs/$name-install.log" | tee -a "$results" > /dev/null
  check "install --silent --parts=$parts exits 0 (ND-293)" test "$rc" -eq 0

  # The app, the uninstall app and the Desktop alias (ND-269, ND-275, ND-290, ND-309).
  check "~/Applications/Network Doctor.app with the program, runnable" test -x "$prog"
  check "the app's start file is there, runnable" test -x "$launcher"
  check "the app's Info.plist passes plutil -lint" plutil -lint "$app/Contents/Info.plist"
  check "the app has LSUIElement true and the name Network Doctor" sh -c \
    "[ \"\$(/usr/libexec/PlistBuddy -c 'Print :LSUIElement' '$app/Contents/Info.plist')\" = true ] &&
     [ \"\$(/usr/libexec/PlistBuddy -c 'Print :CFBundleName' '$app/Contents/Info.plist')\" = 'Network Doctor' ]"
  check "the start file runs the program with open" grep -q 'netdoctor" open$' "$launcher"
  check "GUIDE.md beside the program" test -f "$app/Contents/MacOS/GUIDE.md"
  check "choices.json says scheduled $sched, proactive $pro" grep -qx \
    "{\"core\":true,\"scheduled\":$sched,\"proactive\":$pro,\"ookla\":false}" "$app/Contents/MacOS/choices.json"
  check "~/Applications/Uninstall Network Doctor.app is there" test -x "$uapp/Contents/MacOS/Uninstall Network Doctor"
  check "the Desktop alias or link is there" sh -c "[ -e '$alias_path' ] || [ -L '$alias_path' ]"
  ls -la "$HOME/Applications" "$HOME/Desktop" 2>&1 | sed 's/^/        /' | tee -a "$results" > /dev/null

  # The weekly agent (ND-282).
  if [ "$sched" = true ]; then
    check "weekly LaunchAgent passes plutil -lint" plutil -lint "$wplist"
    check "weekly agent runs the program with weekly" plist_is "$wplist" "ProgramArguments:1" weekly
    check "weekly agent's calendar is Weekday 1, Hour 9, Minute 0" sh -c \
      "[ \"\$(/usr/libexec/PlistBuddy -c 'Print :StartCalendarInterval:Weekday' '$wplist')\" = 1 ] &&
       [ \"\$(/usr/libexec/PlistBuddy -c 'Print :StartCalendarInterval:Hour' '$wplist')\" = 9 ] &&
       [ \"\$(/usr/libexec/PlistBuddy -c 'Print :StartCalendarInterval:Minute' '$wplist')\" = 0 ]"
    check "launchctl print $dom/com.networkdoctor.weekly: loaded" loaded com.networkdoctor.weekly
  else
    check "no weekly agent" not test -e "$wplist"
  fi

  # The alerts agent (ND-285).
  if [ "$pro" = true ]; then
    check "alerts LaunchAgent passes plutil -lint" plutil -lint "$aplist"
    check "alerts agent runs the program with watch" plist_is "$aplist" "ProgramArguments:1" watch
    check "alerts agent has RunAtLoad true and KeepAlive true" sh -c \
      "[ \"\$(/usr/libexec/PlistBuddy -c 'Print :RunAtLoad' '$aplist')\" = true ] &&
       [ \"\$(/usr/libexec/PlistBuddy -c 'Print :KeepAlive' '$aplist')\" = true ]"
  else
    check "no alerts agent" not test -e "$aplist"
  fi

  if [ "$name" = neither ]; then
    open_app_proof
  fi
  page_proof "$name"

  # The weekly job once, through launchd (ND-310).
  # The job's own result line, in the run log its agent names, says whether it saved a row: on a
  # machine that cannot measure the speed it saves none, on purpose.
  if [ "$sched" = true ]; then
    local wlog lb wrc ok=1
    before="$(rows)"
    wlog="$(/usr/libexec/PlistBuddy -c 'Print :StandardOutPath' "$wplist" 2> /dev/null)"
    lb=0
    [ -n "$wlog" ] && [ -f "$wlog" ] && lb="$(wc -l < "$wlog" | tr -d ' ')"
    launchctl kickstart "$dom/com.networkdoctor.weekly" > "$out/logs/$name-kickstart.log" 2>&1
    check "launchctl kickstart of the weekly job is accepted" test "$?" -eq 0 || ok=0
    if [ -z "$wlog" ]; then
      fail "the weekly agent keeps no run log (no StandardOutPath), so its result cannot be read"
      ok=0
    else
      weekly_new() { tail -n +$((lb + 1)) "$wlog" 2> /dev/null > "$out/logs/$name-weekly-run.log"; [ -n "$(weekly_said "$out/logs/$name-weekly-run.log")" ]; }
      check "the weekly job run by launchd printed its result line in its run log (ND-310)" waitfor 300 weekly_new || ok=0
      weekly_new
      sed 's/^/        /' "$out/logs/$name-weekly-run.log" | tail -n 5 | tee -a "$results" > /dev/null
      waitfor 30 sh -c "launchctl print '$dom/com.networkdoctor.weekly' | grep -q 'state = not running'"
      wrc="$(launchctl print "$dom/com.networkdoctor.weekly" 2> /dev/null | sed -n 's/.*last exit code = \([0-9-]*\).*/\1/p' | head -n 1)"
      check "launchd says the weekly job ended well: 0, or the finding's code 10 to 14 after a saved row (got ${wrc:-none}) (ND-310)" \
        weekly_rc_ok "${wrc:-x}" "$out/logs/$name-weekly-run.log" || ok=0
      weekly_rows_check "$out/logs/$name-weekly-run.log" "$before" "$(rows)" "the weekly job run by launchd" || ok=0
    fi
    if [ "$ok" -eq 0 ]; then
      {
        echo "        --- evidence for com.networkdoctor.weekly"
        sed 's/^/          /' "$wplist" 2>&1
        launchctl print "$dom/com.networkdoctor.weekly" 2>&1 | sed 's/^/          /'
        [ -n "$wlog" ] && tail -n 20 "$wlog" 2>&1 | sed 's/^/          run log: /'
      } | tee -a "$results" > /dev/null
    fi
  fi

  # The alerts helper, started as log-in starts it: launchd loads the agent (RunAtLoad).
  if [ "$pro" = true ]; then
    f0=$fails
    before="$(alert_lines)"
    if loaded com.networkdoctor.alerts > /dev/null 2>&1; then
      # Started before the test value was set: taken out so it starts again with it.
      note "the alerts agent was already loaded at install; it is unloaded and loaded again"
      launchctl bootout "$dom/com.networkdoctor.alerts" > /dev/null 2>&1
    fi
    # proactive: a working screen alert (the osascript stand-in first on PATH); both: no osascript
    # on PATH at all (/usr/bin left out; the helper's other tools are in /sbin and /usr/sbin).
    mode=works
    hpath="$screen_stub:/usr/bin:/bin:/usr/sbin:/sbin"
    if [ "$name" = both ]; then
      mode=missing
      hpath="/bin:/usr/sbin:/sbin"
    fi
    : > "$screen_calls"
    runlog="$(/usr/libexec/PlistBuddy -c 'Print :StandardOutPath' "$aplist" 2> /dev/null)"
    log_before=0
    [ -n "$runlog" ] && log_before="$(grep -cF "$nd288" "$runlog" 2> /dev/null || true)"
    launchctl setenv NETDOCTOR_TEST_ALERT 1
    launchctl setenv PATH "$hpath"
    launchctl bootstrap "$dom" "$aplist" > "$out/logs/$name-bootstrap.log" 2>&1
    check "launchctl bootstrap of the alerts agent, as at log-in" test "$?" -eq 0
    check "launchctl print $dom/com.networkdoctor.alerts: loaded" loaded com.networkdoctor.alerts
    check "the alerts helper process is running" waitfor 20 pgrep -f 'MacOS/netdoctor watch$'
    launchctl unsetenv NETDOCTOR_TEST_ALERT
    launchctl unsetenv PATH
    say "  screen case: $mode (helper PATH $hpath)"
    if waitfor 60 more_alerts "$before"; then
      pass "the alerts helper wrote one more line to alerts.log (ND-311)"
      tail -n 3 "$results_dir/alerts.log" | sed 's/^/        /' | tee -a "$results" > /dev/null
    else
      fail "no new line in alerts.log within 1 minute (ND-311)"
    fi
    check "the new line is the test alert, kind test, with its words" sh -c \
      "tail -n +$((before + 1)) '$results_dir/alerts.log' | grep -F '	test	' | grep -qF '$test_words'"
    # The screen alert, read from the stand-in's record and the run log the agent names.
    if [ "$mode" = works ]; then
      waitfor 20 grep -qF "$test_words" "$screen_calls"
    elif [ -n "$runlog" ]; then
      waitfor 40 grep -qF "$nd288" "$runlog"
    fi
    sleep 2
    calls="$(grep -cF "$test_words" "$screen_calls" 2> /dev/null)"
    sed 's/^/        screen call: /' "$screen_calls" | tee -a "$results" > /dev/null
    said=0
    if [ -n "$runlog" ]; then
      tail -n 8 "$runlog" 2> /dev/null | sed 's/^/        /' | tee -a "$results" > /dev/null
      check "the run log shows the test alert" grep -qF "$test_words" "$runlog"
      said=$(( $(grep -cF "$nd288" "$runlog" 2> /dev/null || true) - log_before ))
    else
      fail "the alerts agent keeps no run log (no StandardOutPath), so the plain line of ND-288 cannot be read"
    fi
    if [ "$mode" = works ]; then
      check "osascript was called exactly once with the test alert (calls: ${calls:-0})" test "${calls:-0}" -eq 1
      check "the run log has no plain line of ND-288, since the screen alert worked" test "$said" -eq 0
    else
      check "with no osascript, the run log says the plain line of ND-288 exactly once (said: $said)" \
        test "$said" -eq 1
      check "no screen alert was made (stand-in calls: ${calls:-0})" test "${calls:-0}" -eq 0
    fi
    if [ "$fails" -gt "$f0" ]; then
      {
        echo "        --- evidence for com.networkdoctor.alerts"
        sed 's/^/          /' "$aplist" 2>&1
        launchctl print "$dom/com.networkdoctor.alerts" 2>&1 | sed 's/^/          /'
        [ -n "$runlog" ] && tail -n 20 "$runlog" 2>&1 | sed 's/^/          run log: /'
      } | tee -a "$results" > /dev/null
    fi
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
    # ND-316: an undo note for a fix never applied, naming a proxy server that is not this Mac's.
    # On a Mac the proxy undo may ask for administrator rights, so removal must not try it at all:
    # nothing is changed, the removal exits 1 and only the undo notes stay.
    mkdir -p "$results_dir"
    printf '%s' '{"proxy":"{\"enabled\":true,\"server\":\"proof.invalid:1\"}"}' > "$results_dir/fix-saved.json"
    want_rc=1
  fi
  say "  ${un_args[*]}"
  rm -f /tmp/nd-un-rc /tmp/nd-410-body
  ("$prog" "${un_args[@]}" > "$out/logs/$name-uninstall.log" 2>&1; echo $? > /tmp/nd-un-rc) &
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
  check "the app is gone" not test -e "$app"
  check "the uninstall app is gone" not test -e "$uapp"
  check "the Desktop alias is gone" gone_path "$alias_path"
  check "both LaunchAgent files are gone" not test -e "$wplist" -o -e "$aplist"
  check "launchd holds neither job" sh -c "! launchctl print '$dom/com.networkdoctor.weekly' > /dev/null 2>&1 &&
    ! launchctl print '$dom/com.networkdoctor.alerts' > /dev/null 2>&1"
  if [ "$removal" = keep ]; then
    check "results folder kept with --keep-results" test -d "$results_dir"
  elif [ "$removal" = planted ]; then
    sed 's/^/        /' "$out/logs/$name-uninstall.log" | tee -a "$results" > /dev/null
    check "the removal said the change could not be put back" grep -q 'could not put back' "$out/logs/$name-uninstall.log"
    check "and said why: administrator rights, never asked for" grep -q 'administrator rights' "$out/logs/$name-uninstall.log"
    check "only the undo notes are left in the results folder" sh -c \
      "test -f '$results_dir/fix-saved.json' && [ -z \"\$(ls -A '$results_dir' | grep -v -x -e fix-saved.json -e fix-log.txt -e fix-enabled-here.txt)\" ]"
    rm -rf "$results_dir" # the planted note is the test's own; the next choice starts clean
  else
    check "results folder and settings deleted (${un_args[*]})" not test -e "$results_dir"
  fi
}

run_choice neither "" false false keep
run_choice scheduled "scheduled" true false planted
run_choice proactive "proactive" false true keep
run_choice both "scheduled,proactive" true true old

say "== Done: $fails failed check(s)."
[ "$fails" -eq 0 ]
