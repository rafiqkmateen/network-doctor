# Network Doctor

Purpose: what Network Doctor is and how to build it.
Owner: Rafiq. Version 0.1.0.

A standalone, read-only network checker for wired and Wi-Fi connections. It explains in plain words what is wrong and what to check first. One file, no install. Written in Go.

- Use it: see [docs/GUIDE.md](docs/GUIDE.md).
- What it must do: [docs/requirements.md](docs/requirements.md). How it is checked: [docs/test-cases.md](docs/test-cases.md). Results: [docs/test-results.md](docs/test-results.md).
- Waiting for a decision: [docs/change-list.md](docs/change-list.md).

Build and test:

```
go test ./...
go build ./cmd/netdoctor
```

Make the release files: `powershell -File scripts\package.ps1`.
