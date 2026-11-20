# Network Doctor

Network Doctor is a finished home network checker for people who have never opened a terminal. You press an icon, a colorful page opens in your browser, and it checks your internet connection, wired or Wi-Fi. It explains in plain words what is wrong and what to check first. It never changes your network.

Designed and directed by Rafiq, built with Claude.

This repository is a showcase of the finished tool: its guide, pictures, written requirements, test cases and test results.

## How it works for the person using it

- **Press an icon.** No typing and no commands.
- **A colorful page opens.** It runs a full check by itself and shows the answer in a green box when all is well, or a red box that names the likely cause and what to check first.
- **It explains, it does not change.** Speed, reply time, lost data, Wi-Fi signal and recent drops are shown in plain English. Nothing on the page changes a setting.
- **Devices on your network.** Each device can be marked as mine, not mine, or ignore. Those choices are the only thing the page saves, apart from the new-device switch.
- **A warning for new devices.** When a device it has not seen before joins your network, it tells you.

![Example data: the Network Doctor page when the connection is fine. The answer box is green.](docs/images/web-check-good.png)

*Example data. These numbers are an example, not a real check.*

![Example data: the Network Doctor page when there is a problem. The answer names the cause and what to check first.](docs/images/web-check-problem.png)

*Example data. These numbers are an example, not a real check.*

![Example data: the devices on the network, each marked mine, not mine or ignore.](docs/images/web-devices.png)

*Example data. These devices are an example, not a real network.*

![Example data: the notice an older page shows when a newer Network Doctor page has been opened.](docs/images/web-newer-page.png)

*Example data. This is an example, not a real check.*

## What it will never do

- **It never changes your network.** Checks only look.
- **It sends none of your data away.** No tracking and no reports. Your devices, your results and your settings stay on your computer. The few small requests it makes, such as the speed test, are listed in the [guide](docs/GUIDE.md), with the server each one goes to.
- **It never asks for a router password.**

## What this project demonstrates

| Skill | Evidence in this project |
|---|---|
| **Security by design** | Read-only checks. The page has no button that changes your network, writes data as text only and loads nothing from outside the computer. |
| **Privacy** | No telemetry. Every outside request is named in the guide, and a leak test scans the project for private data. |
| **Engineering discipline** | Written [requirements](docs/requirements.md), [test cases](docs/test-cases.md) and [test results](docs/test-results.md), with a test case for each requirement. |
| **One codebase, every system** | Written in Go, for Windows, Mac and Linux. |
| **Directing AI to build** | Designed and directed by Rafiq, built with Claude, under written rules, a test for each requirement and a separate reviewer. |

## How it is made

Every feature starts as a written requirement. Each requirement has a test case, written before the feature is built, and nothing is called done until its test has been run and the result recorded. The [requirements](docs/requirements.md), [test cases](docs/test-cases.md) and [test results](docs/test-results.md) are published here as they stand, including the rows that are only partly proven. The [guide](docs/GUIDE.md) describes every check and where each result shows.

## Terms

Versions 0.1.0 to 0.3.0 were published earlier under the MIT license and stay MIT. The code is now private and no downloads are offered here. See [LICENSE](LICENSE).

## About the builder

I'm Rafiq. I design and direct software and build it with AI, with written requirements, a test for each, and nothing called done until it is shown to work. My other projects are on [my profile](https://github.com/rafiqkmateen). I am looking for AI implementation, AI security and AI automation roles.
