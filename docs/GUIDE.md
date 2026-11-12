# Network Doctor: one-page guide

Purpose: how to use Network Doctor, in plain words, for anyone.
Owner: Rafiq. Version 0.3.2 (0.3.2: the Start it section points to the installer; 0.3.1: check time; 0.3.0: setup helper, share command, alerts file, Mac and Linux; 0.2.0: install section; 0.1.0: first guide).

## What it does

Network Doctor looks at your internet connection, wired or Wi-Fi. It tells you in plain words what is wrong and what to check first. It only looks. It never changes a setting, a driver or a program.

## Start it

The plain file needs no install (to install it with the weekly check and alerts, see "Install it (Windows)" below). Open a terminal in the folder with `netdoctor.exe` and type:

```
netdoctor check
```

It takes under a minute. It prints the answer first, then the numbers.

The first time you type just `netdoctor` in a window, a short helper asks a few questions: your plan speed, your name, which devices on your network are yours, and whether to turn on the weekly check, the alerts file and the phone page. Press Enter to skip any question. Enter always means no. You can run it again any time with `netdoctor setup`.

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

Open `http://127.0.0.1:8765/` on the same computer.

To open it on your phone, type this instead:

```
netdoctor share
```

It finds this computer's address and starts the page for your own network only. It prints the address and a code. Point the phone camera at the code, or type the address. The phone must be on the same Wi-Fi as the computer. When the phone opens the page, the window says so. If nothing happens after a minute, it lists what to try: press Allow if Windows asks, avoid a guest network, and turn off a VPN. On a light screen, type `netdoctor share --light` so the code is drawn dark on light.

Network Doctor never changes your firewall. Windows may ask once whether to allow it. That choice is yours.

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

It tells you when the link drops, when speed stays low, and when a new device appears. Messages show in the window by default. `netdoctor config quiet 22 6` keeps it quiet from 2200 to 0600.

To save alerts in a file, type `netdoctor config alerts-file on`. Type `off` to stop. It does not change where else alerts go. `netdoctor config sinks console,file,screen` sets all the places at once, with a Windows pop-up as `screen`.

The file is `alerts.log`, in the same folder as `weekly.csv`. Each alert is one line with three parts split by a tab: the time (like `2026-11-11T20:00:00Z`), the kind (`drop`, `slow` or `new_device`), and the message. Another program can read it line by line.

## Install it (Windows)

If you were given `NetworkDoctor-Setup-0.2.0.exe`, double-click it. Windows asks for permission once. Then pick the parts you want:

- **Check on demand.** Always included. You run a check any time you want one.
- **Weekly check.** Once a week, Monday at 0900, it saves your speed to a file. Nothing leaves your computer.
- **Alerts.** A small helper starts when you log on and runs all the time. It tells you on screen when your connection drops, stays slow, or a new device appears. Nothing leaves your computer.

The program goes in `C:\Program Files\Network Doctor`. Your results go in `C:\ProgramData\NetworkDoctor`.

**Run the setup again to repair.** It puts back the parts you chose and removes the ones you did not. A small file, `choices.json`, in the program folder records your choices.

**Better speed numbers (optional).** The setup can offer Ookla's own Speedtest tool, the one behind speedtest.net. It is not part of Network Doctor. Ookla allows it for your own personal use on a computer you own. The setup asks first, shows links to Ookla's license, and only downloads it if you tick that you accept it. You can also install it yourself from Ookla's page and put `speedtest.exe` in an `ookla` folder inside the program folder. Without it, Network Doctor uses its own lighter test. A new speed test waits 2 minutes after an Ookla test and 10 minutes after the other one; press too soon and the card tells you when to come back.

**For IT, with no window.** Run the setup file with `/VERYSILENT /SUPPRESSMSGBOXES`. Add `/COMPONENTS="scheduled,proactive"` for both extras, or just one of the two names. Leave `/COMPONENTS` out to install only the check on demand.

**To remove it.** Open Windows Settings, then Apps, then Installed apps, find Network Doctor and press Uninstall. It asks if you want to keep your saved results. It removes the weekly task and the alerts helper too. IT can run `"C:\Program Files\Network Doctor\unins000.exe" /VERYSILENT /SUPPRESSMSGBOXES`. That keeps your results. Add `/DELETERESULTS=1` to delete them.

## If something goes wrong

- **It says it could not read something**: Windows, Mac and Linux each read your adapter, drops, devices and settings, but a few facts need administrator rights on Mac and Linux, such as the networks near you. Then it says so and does not guess.
- **A number looks wrong**: run `netdoctor check --json` and keep the output. It has every number.
- **Exit codes**: 0 healthy, 10 this computer, 11 cable or signal, 12 router, 13 provider, 2 a bad command.

## Builder box

Files live in `C:\ProgramData\NetworkDoctor` for a copy installed by setup, in `%AppData%\NetworkDoctor` for the plain unzipped file, or in the folder named by the `NETDOCTOR_DIR` setting. Source: Go, `go build ./cmd/netdoctor`. Tests: `go test ./...`. No data leaves the machine except the speed test (Cloudflare's test server) and a ping to 1.1.1.1.
