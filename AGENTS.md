# AGENTS.md

This repository is developed with Codex. Read this file before making any change.

## Hard Safety Boundary: No Automated WeChat Sending

Do not explore, design, implement, test, install, or enable any feature that sends WeChat messages automatically.

This is a permanent project rule.

Background:

- On 2026-05-19, technical exploration tried several WeChat automation/RPA-style approaches.
- The last RPA attempt successfully triggered a send action, but the message went to the wrong chat instead of File Transfer Assistant.
- On 2026-05-20, the account received an official WeChat warning and had to complete learning and an exam before being unblocked.
- Automated WeChat sending is forbidden by WeChat policy and is out of scope for this project.

Forbidden work:

- No WeChat auto-send feature.
- No WeChat auto-reply feature.
- No RPA, UI automation, coordinate clicking, keyboard simulation, clipboard automation, UIA, accessibility automation, Hook, protocol, plugin, or browser bridge for sending WeChat messages.
- No "user confirms, then the program presses Enter/clicks Send" flow.
- No auto-opening a WeChat chat, auto-searching contacts/groups, or auto-filling the WeChat input box.
- No installing or testing WeChat sending packages such as `wxautox4`, `wechat-pc-auto`, `wxauto`, `wechat-auto`, or similar tools.
- No workaround that causes this project to write into WeChat.

Allowed work:

- Read local WeChat records only when explicitly within the documented privacy boundary.
- Store and search allowed messages locally.
- Generate summaries, todos, reminders, reply suggestions, and reply drafts.
- Show drafts in the local UI.
- Let the user manually copy, edit, and send text inside the official WeChat client.

If a task seems to require sending a WeChat message, stop and redirect it to draft generation or local display only.

## Product Direction

`we-flow` is a local WeChat reading and AI analysis workspace.

The intended path is:

1. Whitelisted conversation reading.
2. Local SQLite storage.
3. Local web UI for browsing and search.
4. AI summaries, todos, reminders, and reply drafts.
5. Privacy controls, redaction, and safe logs.

## Privacy Rules

WeChat data is highly sensitive.

- Do not commit raw chat content, contact names, group names, WeChat IDs, database keys, exported records, or local WeChat config.
- Do not print full chat content in ordinary logs.
- Prefer whitelisted conversations and minimal data retention.
- Keep generated reports and examples anonymized unless the user explicitly asks otherwise.

## Implementation Notes

- Prefer the existing Go backend under `cmd/` and `internal/`, and the local web UI under `web/`.
- Keep changes small and scoped.
- Use `rg` for search.
- Use `apply_patch` for manual file edits.
- Do not revert unrelated user changes.

## Related Docs

- `docs/工作原则.md`
- `docs/2026-05-20-自动发送微信终止记录.md`
- `docs/微信发送能力验证报告.md`
- `docs/下一步验证计划.md`
