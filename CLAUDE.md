# CLAUDE.md

Purpose: the instructions for working on Network Doctor, and the list of standards this project follows.
Owner: Rafiq. Version 0.1.0. Date: 2026-11-11.

Network Doctor is a standalone, read-only network checker for wired and Wi-Fi connections, written in Go, shipped as one file per system (Windows, Mac, Linux, ARM servers). It is its own project. Khadijah may call it later; it never depends on Khadijah.

## Rules from the global file apply

Plan agreed first. Requirements fixed first. A test for every requirement, written before the build. Nothing is done until checked against its requirement with the result shown. Every document has a purpose, an owner and a version. Every change is recorded and versioned.

## Standards

Follow these. The plan confirms each edition year by lookup.

- ISO/IEC/IEEE 12207 (lifecycle) with 24748-3 (guidance on applying it), 29148 (requirements), 42010 (architecture description), 25010 (quality traits), 29119 (testing), 15289 and 26514 (documents), 29110 (small-team lifecycle), 5055 (code quality measures).
- Go: gofmt, go vet, Effective Go, Go Code Review Comments.
- Semantic Versioning, Keep a Changelog, Conventional Commits.

## Hard rules for this project

1. Read-only first: version 1 never changes a setting, driver, device or program.
2. Stage 2 (apply a fix) only with a per-fix approval, a before and after view, one-press undo and a change log. Never drivers, security protections, or anything that could cut off the connection in use.
3. No telemetry. Nothing leaves the machine unless the user turns it on.
4. Any text shown to people is plain English at a 6th to 8th grade level.
5. The repository stays private until Rafiq says to make it public. The license is Rafiq's choice.
6. Router logins are typed by the user; never guessed, stored or logged.
