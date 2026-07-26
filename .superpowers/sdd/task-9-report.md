# Phase 1 Task 9 Implementation Report

## Status

- Status: `DONE_WITH_CONCERNS`
- Base commit: `47feec207c7484d589a430860f1f1f3a6b0d0336`
- Implementation commit: `7109950853bb33ed28a2ad71dafb5dec1b4b3c2d`
- Commit subject: `feat: add server and agent setup ui`
- Scope: Phase 1 Task 9 only. Task 10 application composition, container files, load generator, and real Agent binaries were not implemented.

The implementation and all required automated verification are complete. The only concern is that the in-app browser runtime reported no available browser, so a live visual screenshot pass could not be completed. Responsive, focus, touch-target, contrast, and overflow behavior are covered by automated DOM/CSS tests instead.

## Delivered behavior

### Public Agent downloads

- Added `AgentDownloadHandler(fs.FS)` with an exact allowlist for:
  - `/downloads/tinyprobe-agent-linux-amd64`
  - `/downloads/tinyprobe-agent-linux-arm64`
- Unknown names and directory-style paths return `404`; no directory listing is exposed.
- Successful responses use `Content-Disposition: attachment; filename="..."` and serve the file bytes.
- Routes are outside session authentication.
- `nil` or missing Task 10 file systems return safe `404` responses rather than panicking.
- No fake Agent binaries were created.

### Typed server-management API

- Added typed list, create, update, delete, and rotate-token requests.
- Create and rotate responses are modeled as `{ server, token }`.
- The rotate response's `server` record is consumed to refresh the managed row.
- Existing validated API error handling is reused for server-provided and friendly fallback messages.

### Server management

- Added `/servers` management routing and navigation while preserving `/servers/{id}` detail routing.
- Added inline server creation, rename, enable/disable, deletion confirmation, and token-rotation confirmation.
- No modal dialogs are used.
- Successful enable/disable mutations update the row from the response without refetching the list.
- Delete confirmation explains immediate Agent Token invalidation and database-cascade removal of live and historical data.
- Rotation confirmation explains immediate invalidation of the current token.
- Loading, empty, failure, busy, disabled, and retry states are explicit.
- Creation is disabled while list/reload data is pending, preventing stale list responses from overwriting a newly created row.

### One-time token and Agent installation

- Raw tokens live only in React component state.
- They are rendered only after create or rotation.
- `我已保存 Token` removes the token and all generated token-bearing commands from the DOM.
- Tokens are never written to local storage, session storage, or the URL.
- Token state tracks server IDs, so duplicate server names cannot clear another server's token.
- Added accessible `amd64` and `arm64` tabs with click and arrow/Home/End keyboard operation.
- All generated URLs use `window.location.origin`.
- Added copyable download command, env-file content, installation command sequence, and systemd unit.
- The env file is created with mode `0600` before token content is written.
- The service uses `DynamicUser=true`, `NoNewPrivileges=true`, `ProtectSystem=strict`, and read-only `/proc` and `/sys` access.
- Copy success uses an accessible status message; copy failure uses an accessible alert.

### Visual and responsive integration

- Preserved the existing white, pale-rose, deep-rose, system-sans design language.
- Added no glassmorphism, gradient text, heavy shadows, oversized rounding, side-stripe accents, or nested decorative card stacks.
- Interactive controls retain at least 44 px targets.
- Management rows/actions and installation headings stack on mobile.
- Long command blocks are locally scrollable and bounded to the page width.
- Focus-visible styling is provided for tabs, rename fields, code blocks, buttons, and navigation.
- Existing global `prefers-reduced-motion` handling applies to the new UI.
- Destructive buttons use the darker danger token, verified at at least 4.5:1 contrast against white text.

## Files changed

### Go server

- `internal/httpapi/downloads.go`
- `internal/httpapi/downloads_test.go`
- `internal/httpapi/router.go`

### Frontend API and routing

- `web/src/api/client.ts`
- `web/src/servers/api.ts`
- `web/src/servers/api.test.ts`
- `web/src/app/App.tsx`
- `web/src/app/App.test.tsx`
- `web/src/components/AppNav.tsx`

### Management and installation UI

- `web/src/components/ServerForm.tsx`
- `web/src/components/ServerInstallPanel.tsx`
- `web/src/components/ServerInstallPanel.test.tsx`
- `web/src/pages/ServersPage.tsx`
- `web/src/pages/ServersPage.test.tsx`

### Styling and UI contracts

- `web/src/styles/dashboard.css`
- `web/src/styles/accessibility.test.ts`
- `web/src/styles/management-responsive.test.ts`

### Rebuilt Go embed

- `internal/webui/dist/index.html`
- `internal/webui/dist/assets/index-Csf9UGgG.css`
- `internal/webui/dist/assets/index-CyfqFSNe.js`
- Removed superseded Task 8 bundles.

## TDD evidence

All production behavior was introduced after a focused failing test.

### 1. Download handler and router wiring

RED:

```text
go test ./internal/httpapi -run AgentDownload -v
internal/httpapi/downloads_test.go:29:12: undefined: httpapi.AgentDownloadHandler
internal/httpapi/downloads_test.go:57:11: undefined: httpapi.AgentDownloadHandler
internal/httpapi/downloads_test.go:69:51: unknown field AgentFiles in struct literal of type httpapi.Dependencies
FAIL probe.local/monitor/internal/httpapi [build failed]
```

GREEN:

```text
go test ./internal/httpapi -run AgentDownload -v
PASS
ok probe.local/monitor/internal/httpapi
```

The green run covered both allowlisted names, bytes, attachment filenames, unknown names, public router access, and nil-file `404` behavior.

### 2. Typed server API

RED:

```text
pnpm --dir web test -- --run src/servers/api.test.ts
FAIL src/servers/api.test.ts
Error: Failed to resolve import "./api"
```

GREEN:

```text
Test Files 1 passed (1)
Tests 2 passed (2)
```

The tests assert all five endpoint contracts and preserve `{ server, token }` on rotation.

### 3. Installation panel

Initial RED:

```text
pnpm --dir web test -- --run src/components/ServerInstallPanel.test.tsx
FAIL: Failed to resolve import "./ServerInstallPanel"
```

Initial GREEN:

```text
Test Files 1 passed (1)
Tests 5 passed (5)
```

Keyboard RED:

```text
Expected arm64 tab to have focus after ArrowRight.
Received focus on amd64.
Tests 1 failed | 5 skipped
```

Keyboard GREEN:

```text
Test Files 1 passed (1)
Tests 6 passed (6)
```

The final panel tests cover current-origin commands, token/env content, architecture switching, all required systemd directives, copy success/failure announcements, token acknowledgement, and keyboard tabs.

### 4. Management page and mutation flows

Initial RED:

```text
pnpm --dir web test -- --run src/pages/ServersPage.test.tsx
FAIL: Failed to resolve import "./ServersPage"
```

GREEN after implementation and accessible status wrappers:

```text
Test Files 1 passed (1)
Tests 12 passed (12)
```

This covered list/create/rename/enable/disable/delete, inline confirmations, rotate confirmation, one-time token disappearance, no storage/URL persistence, and server/fallback errors for every operation.

Duplicate-name regression RED:

```text
Unable to find an element with the text: tp_duplicate
Tests 1 failed | 12 skipped
```

Duplicate-name regression GREEN:

```text
Test Files 1 passed (1)
Tests 1 passed | 12 skipped (13)
```

List/create race RED:

```text
Expected 添加服务器 to be disabled while the list promise was pending.
Received element is not disabled.
```

List/create race GREEN:

```text
Test Files 2 passed (2)
Tests 2 passed | 18 skipped (20)
```

### 5. App route and navigation

RED:

```text
Unable to find role="heading" and name "服务器管理"
The /servers path rendered Overview and only the 概览 nav item.
```

GREEN:

```text
Test Files 1 passed (1)
Tests 1 passed | 6 skipped (7)
```

### 6. Responsive and accessibility CSS

Responsive RED:

```text
Test Files 1 failed (1)
Tests 3 failed (3)
Missing touch-size, overflow containment, and mobile stacking contracts.
```

Responsive GREEN:

```text
Test Files 1 passed (1)
Tests 3 passed (3)
```

Destructive contrast RED:

```text
Expected .button-danger to use var(--color-danger-text); it used the brighter danger token.
Tests 1 failed | 2 skipped
```

Destructive contrast GREEN:

```text
Test Files 1 passed (1)
Tests 1 passed | 2 skipped
```

### 7. Security review corrections

RED:

```text
Expected install commands to contain:
sudo install -m 0600 /dev/null /etc/tinyprobe-agent.env

Expected 添加服务器 to be disabled while server list was pending.

Test Files 2 failed (2)
Tests 2 failed | 18 skipped (20)
```

GREEN:

```text
Test Files 2 passed (2)
Tests 2 passed | 18 skipped (20)
```

The same panel test also requires `DynamicUser=true` in the unit.

## Final verification

Executed from `C:\Project\baby-diary\.worktrees\probe-implementation` after the final fixes.

### Frontend tests

Command:

```text
pnpm --dir web test -- --run
```

Result:

```text
Test Files 12 passed (12)
Tests 76 passed (76)
Exit code 0
```

### TypeScript lint/check

Command:

```text
pnpm --dir web lint
```

Result:

```text
tsc -b --pretty false
Exit code 0
```

### Production build / embedded UI

Command:

```text
pnpm --dir web build
```

Result:

```text
47 modules transformed
internal/webui/dist/assets/index-Csf9UGgG.css
internal/webui/dist/assets/index-CyfqFSNe.js
Exit code 0
```

### Go tests

Command:

```text
C:\Program Files\Go\bin\go.exe test ./...
```

Result: all Go packages passed; packages without tests were reported normally; exit code `0`.

### Go vet

Command:

```text
C:\Program Files\Go\bin\go.exe vet ./...
```

Result: no findings; exit code `0`.

### Diff whitespace check

Command:

```text
git diff --check
```

Result: exit code `0`. Git emitted only the repository's existing Windows LF-to-CRLF conversion warnings; no whitespace errors were reported.

## Independent review

The first review reported four Important issues:

1. stale embedded bundle after the duplicate-name fix;
2. env file defaulting to world-readable permissions;
3. systemd service running as root;
4. list/create response race.

Corrections:

- rebuilt `internal/webui/dist` after all source changes;
- added `sudo install -m 0600 /dev/null /etc/tinyprobe-agent.env`;
- added `DynamicUser=true` to the unit and copied unit content;
- disabled creation controls whenever the server list/reload is pending.

Final re-review verdict:

```text
Approved. All four previously reported Important issues are fixed in source and
the rebuilt embedded bundle. No remaining Critical or Important findings.
```

## Self-review

- Download names are exact and handler input cannot select arbitrary filesystem paths.
- Missing Agent files remain safe until Task 10 supplies real binaries.
- Raw tokens are never fetched from list responses or persisted outside memory.
- Duplicate names are handled by server ID for token ownership.
- The installation flow protects the token file at `0600` and runs the collector as a dynamic unprivileged user.
- Delete and rotation are inline, explicit, cancellable, and never triggered by the first click.
- All mutation failures leave the affected row/form available for retry.
- Long generated commands remain inside local scroll containers on narrow screens.
- The tracked Go embed references the final generated asset names.
- No unrelated Task 10 work or old minor findings were included.

## Concern

The required Browser control surface was unavailable:

```text
agent.browsers.getForUrl(...): No browser is available
agent.browsers.list(): []
```

A temporary local server did pass `/api/health` before browser discovery, and all temporary database/log files were removed. No screenshot or live viewport inspection could be produced in this environment.

---

## Quality-review fixes

Fix-turn base: `fdabe41e7b58563a82c9233d04262268152cad9b`.

### Download method safety and streaming

Focused RED:

```text
go test ./internal/httpapi -run 'TestAgentDownloadHandler' -count=1
Content-Type="text/plain; charset=utf-8", want application/octet-stream
TestAgentDownloadHandlerRejectsUnsupportedMethodsBeforeOpeningFile: status=200 body="binary"
FAIL probe.local/monitor/internal/httpapi
```

Focused GREEN after switching to `http.ServeFileFS`, rejecting non-GET/HEAD before filesystem access, and adding attachment/binary headers:

```text
ok probe.local/monitor/internal/httpapi 1.015s
```

Coverage now includes HEAD, `405` plus `Allow: GET, HEAD`, ranges, missing files, a production-sized in-memory binary, and an aborting response writer that proves the whole file is not read before streaming begins.

### Token-free executable install commands

Focused RED:

```text
Test Files 1 failed (1)
Tests 2 failed | 4 passed (6)
Expected install commands not to contain tp_one_time; raw token was present.
ArrowRight from arm64 did not wrap to amd64.
```

Focused GREEN:

```text
Test Files 1 passed (1)
Tests 6 passed (6)
```

The executable block now uses `read -rsp`, writes through `sudo tee` to a root-created `0600` environment file, and unsets the shell variable. The raw token remains only in the separately displayed one-time value/reference.

### Request generations and list ordering

Initial hook RED:

```text
Failed to resolve import "./useServerCollection"
Test Files 1 failed (1)
```

The new collection hook received deferred-promise tests for independent rows, same-row stale whole-record responses, and list/mutation ordering. A self-review added the missing interleaving where a list begins during a mutation:

```text
Test Files 1 failed (1)
Tests 1 failed | 3 passed (4)
- Expected enabled: false
+ Received enabled: true
```

GREEN after advancing the mutation revision on successful completion as well as request start:

```text
Test Files 1 passed (1)
Tests 4 passed (4)
```

### Token serialization, shell lifecycle, navigation, and focus

Focused page RED established the original failures:

```text
ServersPage: 5 failed
- install heading did not receive focus
- rotation remained enabled during a token-producing request
- rotation acknowledgement did not restore focus
- delayed delete A cleared the newer token for B
- deletion did not focus the surviving row
```

The one-time token and token-request lock now live only in `DashboardRouter` memory. Create/rotation are atomically serialized; deletion uses a functional token setter; pending create/rotation responses publish after page unmount; an unload guard remains active while a request or unsaved token exists; and non-management routes show an inline return reminder.

Additional lifecycle RED/GREEN evidence:

```text
Pending rotation after route remount:
RED: 删除 home-lab was not disabled
GREEN: 1 passed | 8 skipped

Pending create after returning to /servers:
RED: Unable to find [data-testid="managed-server-7"]
GREEN: 1 passed | 8 skipped
```

The shell retains the pending rotation server ID so the full row remains locked across remounts. Publishing a token advances the management-page generation, forcing a fresh list after a deferred create/rotation.

The install heading is programmatically focused. A persistent, initially empty `aria-live="polite"` region is populated asynchronously when each token becomes ready. Acknowledgement restores focus to Add or the originating rotation control; deletion focuses a surviving row or Add.

Persistent live-region RED/GREEN:

```text
RED: Unable to find [data-testid="token-ready-announcement"]
GREEN: 1 passed | 6 skipped
```

### Browser verification

The browser control surface was available for this fix turn. The authenticated create/install flow and the unsaved-token reminder were inspected at desktop and narrow mobile widths. The live accessibility tree showed the install heading focused, the rotation control disabled while the token was unsaved, and the executable block without the raw token. Mobile Lighthouse snapshot scores:

```text
Accessibility: 100
Best Practices: 100
```

### Final verification

```text
pnpm --dir web test -- --run
Test Files 13 passed (13)
Tests 86 passed (86)
Exit code 0

pnpm --dir web lint
tsc -b --pretty false
Exit code 0

pnpm --dir web build
48 modules transformed
internal/webui/dist/assets/index-BnV3jBIA.css
internal/webui/dist/assets/index-CDGWcsni.js
Exit code 0

C:\Program Files\Go\bin\go.exe test ./...
All packages passed; exit code 0

C:\Program Files\Go\bin\go.exe vet ./...
No findings; exit code 0

git diff --check
No whitespace errors; exit code 0
```

Independent review found the list-during-mutation, route-remount rotation lock, streaming-test, live-region, and deferred-create remount gaps during successive passes. Each was reproduced or strengthened with focused coverage and corrected. No Task 10 binaries or behavior were added.
