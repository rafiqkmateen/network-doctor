# Requirements

Purpose: the fixed list of what Network Doctor must do. Each requirement has one test in `test-cases.md`.
Owner: Rafiq. Version 0.1.0 (draft, awaiting Rafiq's agreement). Date: 2026-11-11.

Form: each requirement is one "shall" sentence that can be tested. A new idea goes on the change list, not here.

## Ground rules

- ND-01 The tool shall only read and report in stage 1; it shall not change any setting, driver, device or program.
- ND-02 The tool shall run with no install, as a single executable per system (Windows, Mac, Linux, ARM Linux), also offered as a zip with a one-page guide.
- ND-03 The tool shall work offline except for the speed test and internet checks.
- ND-04 The tool shall keep all saved data on the local machine and shall send nothing anywhere unless the user turns it on.
- ND-05 All text shown to people shall be plain English at a 6th to 8th grade level.

## Checks

- ND-10 Diagnose: the tool shall detect whether the active connection is wired or Wi-Fi.
- ND-11 Diagnose: the tool shall measure speed and delay to the router and to the internet, and packet loss.
- ND-12 Diagnose: the tool shall name the most likely cause (this computer, cable or signal, router, or provider) and what to check first.
- ND-20 Bandwidth: the tool shall list the programs using the connection, ranked, with upload and download.
- ND-21 Bandwidth: the tool shall recommend which activities could be paused, and shall not stop any.
- ND-30 Drops: the tool shall read the last seven days of connection history and report the number of drops, the time of each and the length.
- ND-31 Drops: the tool shall point out patterns (time of day, power-saving, driver).
- ND-40 Link health, wired: the tool shall report link speed, duplex and error counts.
- ND-41 Link health, Wi-Fi: the tool shall report signal strength, band, channel and nearby networks on the same channel, with advice.
- ND-50 Devices: the tool shall list devices on the network with name, maker, address and hardware address, and flag any not marked known.
- ND-51 Devices: the tool shall never guess, store or log a router password.
- ND-60 Settings: the tool shall review DNS, adapter power-saving, VPN, proxies, adapter and driver version and configuration errors, and explain each finding and the effect of changing it.

## Modes

- ND-70 Reactive: the user shall be able to run a full check on demand.
- ND-71 Scheduled: the user shall be able to turn on a weekly check (default Monday 0900) that records speed, delay, date, time and subscribed speed to a spreadsheet file.
- ND-72 Scheduled: if download stays under half the subscribed speed for three consecutive weekly checks, the tool shall prepare a complaint email as a draft and shall never send it.
- ND-73 Proactive: the tool shall notify the user when the link drops, when speed stays below plan, or when a new unknown device appears.
- ND-74 Proactive: each alert type shall be switchable, shall honor quiet hours, and shall give one alert per outage.
- ND-75 Proactive: alerts shall go only where the user chooses; by default nowhere outside the machine.

## Interfaces

- ND-80 The tool shall have a command-line mode with no screen, JSON output and clear exit codes, for servers.
- ND-81 The tool shall serve a local web page for people and phones, reachable only from the local machine or networks the user allows.

## Stage 2 (apply with approval)

- ND-90 The tool shall apply a fix only after the user approves that single fix.
- ND-91 Before applying, the tool shall show the before and after values.
- ND-92 The tool shall save the old value and undo the change with one action.
- ND-93 The tool shall write every applied change to a log.
- ND-94 The tool shall request administrator rights only at the moment of approval.
- ND-95 The tool shall never apply driver changes, security-protection changes, or any change that could cut off the connection in use.
- ND-96 On an unattended server, apply shall be off by default and switchable only per machine by the user.

## Release

- ND-100 Each release shall include checksums and a signature.
- ND-101 Automated tests shall run on Windows, Mac and Linux for every change.

## Later (change list, not in this build)

- A Khadijah node that calls the tool and reads its results.
- A smaller native phone app.
