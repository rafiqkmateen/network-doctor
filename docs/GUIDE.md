# Network Doctor: one-page guide

Purpose: how to use Network Doctor, in plain words, for anyone.
Owner: Rafiq. Version 0.2.0 (0.2.0: install section; 0.1.0: first guide).

## What it does

Network Doctor looks at your internet connection, wired or Wi-Fi. It tells you in plain words what is wrong and what to check first. It only looks. It never changes a setting, a driver or a program.

## Start it

There is nothing to install. Open a terminal in the folder with `netdoctor.exe` and type:

```
netdoctor check
```

It takes about 30 seconds. It prints the answer first, then the numbers.

## What you see

- **Answer**: whether the problem is this computer, the cable or signal, the router, or your provider.
- **Drops**: how many times the connection dropped in the last 7 days, and when.
- **Busiest programs**: what is using the line. It is an estimate, and it never stops anything.
- **Devices**: everything on your network. Mark the ones you know with `netdoctor known add MAC NAME`.
- **Settings review**: six things that often cause trouble, each with what changing it would do.

## See it on a screen or a phone

```
netdoctor serve
```

Open `http://127.0.0.1:8765/` on the same computer. To open it from your phone, start it like this, using your own network range:

```
netdoctor serve --listen 0.0.0.0:8765 --allow 192.168.1.0/24
```

Then open `http://<this computer's address>:8765/` on the phone. Only the addresses you allow can connect.

## Check every week

```
netdoctor config plan 300        (your plan speed, in Mbps)
netdoctor config name "Your Name"
netdoctor schedule enable        (Monday at 0900 by default)
```

Each week it adds a line to `weekly.csv`, which Excel opens. If your download stays under half of your plan for three weeks in a row, it writes a draft complaint email to `complaint-draft.txt`. It never sends it. You read it, change it and send it yourself.

## Get warned

```
netdoctor watch
```

It tells you when the link drops, when speed stays low, and when a new device appears. Messages show in the window by default. `netdoctor config sinks console,file,screen` adds a log file and a Windows pop-up. `netdoctor config quiet 22 6` keeps it quiet from 2200 to 0600.

## Install it (Windows)

If you were given `NetworkDoctor-Setup-0.1.0.exe`, double-click it. Windows asks for permission once. Then pick the parts you want:

- **Check on demand.** Always included. You run a check any time you want one.
- **Weekly check.** Once a week, Monday at 0900, it saves your speed to a file. Nothing leaves your computer.
- **Alerts.** A small helper starts when you log on and runs all the time. It tells you on screen when your connection drops, stays slow, or a new device appears. Nothing leaves your computer.

The program goes in `C:\Program Files\Network Doctor`. Your results go in `C:\ProgramData\NetworkDoctor`.

**For IT, with no window.** Run the setup file with `/VERYSILENT /SUPPRESSMSGBOXES`. Add `/COMPONENTS="scheduled,proactive"` for both extras, or just one of the two names. Leave `/COMPONENTS` out to install only the check on demand.

**To remove it.** Open Windows Settings, then Apps, then Installed apps, find Network Doctor and press Uninstall. It asks if you want to keep your saved results. It removes the weekly task and the alerts helper too. IT can run `"C:\Program Files\Network Doctor\unins000.exe" /VERYSILENT /SUPPRESSMSGBOXES`. That keeps your results. Add `/DELETERESULTS=1` to delete them.

## If something goes wrong

- **It says "not available on this system yet"**: that check is built for Windows first. Speed, delay and drops of the line itself still work on Mac and Linux.
- **A number looks wrong**: run `netdoctor check --json` and keep the output. It has every number.
- **Exit codes**: 0 healthy, 10 this computer, 11 cable or signal, 12 router, 13 provider, 2 a bad command.

## Builder box

Files live in `C:\ProgramData\NetworkDoctor` for a copy installed by setup, in `%AppData%\NetworkDoctor` for the plain unzipped file, or in the folder named by the `NETDOCTOR_DIR` setting. Source: Go, `go build ./cmd/netdoctor`. Tests: `go test ./...`. No data leaves the machine except the speed test (Cloudflare's test server) and a ping to 1.1.1.1.
