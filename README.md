# Network Doctor

Purpose: the front page. What Network Doctor is, what it never does, and how to get and use it.
Owner: Rafiq. Version 0.3.0 (0.3.0: the Windows setup file, Ookla speed, and the plain-zip wording; 0.2.0: public front page; 0.1.0: build notes).

Network Doctor checks your internet connection, wired or Wi-Fi. It tells you in plain words what is wrong and what to check first. It is one small file for Windows, Mac or Linux, and Windows also has a setup file that installs it with the parts you pick.

## What it will never do

- **It only looks.** It never changes a setting, a driver or a program.
- **It sends nothing away.** No tracking and no reports. Everything stays on your computer.
- **It never asks for a router password.** If a check ever needed one, you would type it yourself, and it would not be saved.

## What it tells you

- Whether the trouble is this computer, the cable or signal, your router, or your provider.
- How fast your line is, how long replies take, and how much data gets lost.
- How many times the connection dropped in the last week, and when.
- Which programs are using the line, and which devices are on your network.
- A weekly record of your speed, and a draft note you can send your provider (it is never sent for you).

## Get it

Download the file for your system from the [Releases](../../releases) page. Check it against the `SHA256SUMS.txt` file there. On Windows you can take either the plain zip or `NetworkDoctor-Setup-0.2.0.exe`, which installs it, can add a weekly check and alerts, and can repair or remove itself later. The [one-page guide](docs/GUIDE.md) explains each choice.

The Windows files are not signed yet. A computer with a strict app rule, such as Smart App Control, may refuse to run them. We do not ask you to switch that protection off.

For a more exact speed number, Network Doctor can use Ookla's own Speedtest tool if you put it beside the program yourself. It is never included, and you accept Ookla's license yourself. Without it, Network Doctor uses its own lighter test.

## Use it

Open a terminal in the folder with the file and type:

```
netdoctor check
```

It takes under a minute. More commands are in the [one-page guide](docs/GUIDE.md): a page you can open on your phone, a weekly check, and alerts when the connection drops.

## Build it yourself

You need [Go](https://go.dev/dl/).

```
go test ./...
go build ./cmd/netdoctor
```

## How it is made

- What it must do: [requirements](docs/requirements.md). How each part is checked: [test cases](docs/test-cases.md), with [results](docs/test-results.md).
- Ideas waiting for a decision: [change list](docs/change-list.md). What changed: [changelog](CHANGELOG.md).

## License

MIT. See [LICENSE](LICENSE). One outside library, `rsc.io/qr`, has its own notice in [THIRD-PARTY-NOTICES.txt](THIRD-PARTY-NOTICES.txt).
