# Stage 2 plan: apply a fix with approval

Purpose: the plan for Stage 2, for Rafiq to agree before anything is built. It adds no requirements: ND-90 to ND-96 and their tests already exist.
Owner: Rafiq. Version 0.2.0 (0.2.0: built and agreed 2026-11-12, kept for the record; 0.1.0: draft). Date: 2026-11-12. Change list row 5.

## What Stage 2 is

Network Doctor already finds a problem and says what to check. Stage 2 lets it make a small fix, but only when you say yes to that one fix.

## The rules (already agreed: ND-90 to ND-96)

1. One yes per fix. Nothing changes without it.
2. Before you say yes, it shows the setting now and what it will be.
3. It saves the old value. One press puts it back.
4. Every change and every undo goes in a log: time, setting, old value, new value.
5. It asks for administrator rights only at the moment you say yes, never before.
6. Never drivers, never security protections, never anything that could cut off the connection you are using.
7. On a server nobody sits at, fixes are off until the owner switches them on for that machine.

## Which fixes, in order

| # | Fix | Why it is safe | Needs administrator rights |
|---|-----|----------------|----------------------------|
| 1 | Clear the saved list of website addresses (flush the name cache) | Holds nothing of yours. The computer rebuilds it. Nothing to undo. | No |
| 2 | Turn off a proxy | A per-user switch. The old address is saved and put back by undo. | No |
| 3 | Turn off adapter power saving | Only if a test shows it never drops the link in use. If it can, the fix only says what to change by hand. | Yes |

Not in Stage 2: changing the DNS servers (a wrong one cuts off websites), drivers, VPN, firewall, anything on the router.

## How it is built

- A new `fix` command (for example `netdoctor fix list`, `netdoctor fix apply NAME`, `netdoctor fix undo NAME`). The check itself stays read-only and never runs a fix.
- Each fix is one small piece with four parts: read the setting, show before and after, apply, undo.
- The saved old values and the log live in the tool's own results folder, beside the weekly record.
- A fix is refused if the connection in use depends on it.
- On Windows, fix 3 starts an administrator prompt only at the yes. Mac and Linux get fix 1 and 2 only at first.

## Tests first (all already written down; the Go tests come before the code)

- ND-90, ND-91, ND-92, ND-93, ND-95, ND-96: automated, on a pretend computer.
- ND-94: measured on this laptop, shown to you.
- One new test case for fix 3: the real link stays up through the change (measured by you or me on this laptop, result shown).

## What I need from you

- Your yes to this plan, and which of the three fixes you want. After that: Go tests first, then the code, then results shown to you.
