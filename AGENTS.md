# AGENTS.md

This is a private fork of MetaCubeX/ClashMetaForAndroid (`origin` = LeafAI fork,
`upstream` = MetaCubeX). Changes are **not** sent back upstream, so the goal is to
keep our delta small and easy to re-merge.

## Our changes on top of upstream

| Area | Where |
|---|---|
| Delay results shared across views, kept across mode switches and app restarts, shown live while testing | `native/delaycache/`, `native/tunnel/delay_cache.go`, `proxies.go`, `connectivity.go`; `ProxyActivity.kt`, `ProxyDesign.kt`, `ProxyViewState.kt` |
| `GLOBAL` group always listed, tested directly | `tunnel/proxies.go`, `tunnel/connectivity.go` |
| Global rules (YAML source merged into every profile) | `native/rulemerge/`, `native/config/rules.go`, `RuleSourceActivity.kt`, `RuleSourceDesign.kt`, `design_rule_source.xml` |
| Connections page (every connection since the app started, matched rule and outbound) | `native/connview/`, `native/sampler/`, `native/tunnel/connections.go`, `Connection*.kt`, `ConnectionsActivity.kt`, `ConnectionsDesign.kt`, `design_connections.xml`, `adapter_connection.xml` |
| Shared signing key, Makefile, sync script | `build.gradle.kts`, `Makefile`, `scripts/sync-upstream.sh` |

Upstream files we edit and that upstream also changes often (expect conflicts):
`build.gradle.kts`, `core/src/main/golang/go.mod`, `tunnel/proxies.go`,
`config/process.go`, `ProxyActivity.kt`. Keep edits to them minimal and put new logic
in new files, as `delay_cache.go`, `rules.go` and `rulemerge/` do.

## Syncing upstream

Never merge upstream straight into `main`. Use the merge workflow:

1. `make sync-upstream` (runs `scripts/sync-upstream.sh`). It needs a clean tree on `main`.
2. The script fetches `upstream/main`, lists new commits and the files changed on both
   sides, merges into `sync/upstream-<date>` (signed-off merge commit), then runs
   `make sync-check` (`go vet` + `go test`). `git rerere` is enabled so a repeated
   conflict is only resolved once.
3. On conflicts it stops on the sync branch. For `go.mod` / `go.sum` keep upstream's
   side, then run `go mod tidy` in `core/src/main/golang`. Resolve, `git add`,
   `git merge --continue`, then `make sync-check`.
4. Build and test on a device or emulator (`make android-release`), then fast-forward:
   `git switch main && git merge --ff-only sync/upstream-<date> && git push origin main`.

Why merge and not rebase: our commits are already pushed, and rebasing would rewrite
shared history.

When to sync: after each upstream version bump (`Bump version to ...`). Upstream updates
dependencies every one to two weeks, so waiting longer makes each merge bigger.

After every sync, re-verify on an emulator: delay test then switching views/modes,
`GLOBAL` delay test, a global rule saved in Settings > Global Rules taking effect, and
the Connections page listing, filtering and closing connections.

The `core/src/foss/golang/clash` submodule (mihomo) is pinned by upstream. Do not bump it
ourselves. After a merge run `git submodule update --init --depth 1`.

## Build

- `make android-debug` / `make android-release` — build APKs (`FLAVOR=Meta|Alpha`, default Meta).
  Output: `app/build/outputs/apk/<flavor>/<type>/`.
- `make android-install` — build debug and `adb install` the `ABI` build (default arm64-v8a).
- `make android-verify` — print the signing certificate of built APKs.
- `make go-vet` / `make go-test` — check the native Go packages (`rulemerge`, `connview`,
  `delaycache`, `sampler`; also `make sync-check`).
- `make serveapk` — clean, build release, serve over HTTP on port 9980.
- Requires the Android NDK `29.0.14206865` (the version `build.gradle.kts` pins) in the SDK.
  `Makefile` defaults `ANDROID_HOME` to `~/android-sdk` and `JAVA_HOME` to Android Studio's
  bundled JDK 21; system Java 25 is incompatible with Gradle 8.x.
- The mihomo submodule must be present: `make` runs `git submodule update --init --depth 1`.
- The first build needs network access to GitHub (geo data, mihomo), Maven and Gradle;
  use a proxy if needed.
- CMake reads the mihomo submodule's branch name from git. If it fails with
  `string sub-command REGEX ... needs at least 6 arguments`, the submodule has no
  `origin/Alpha` ref. Create it locally: `git -C core/src/foss/golang/clash update-ref refs/remotes/origin/Alpha HEAD`.

## Signing

- Upstream signs releases with `release.keystore` using `signing.properties`
  (git-ignored, created by upstream CI from secrets). We do not have those passwords,
  so local builds cannot reproduce upstream's signature.
- For our builds put these in `local.properties` (git-ignored, never commit them):
  `signing.keystore=<path to a keystore>`, and optionally `signing.store.password`,
  `signing.key.alias`, `signing.key.password` (default `android` / `androiddebugkey`).
  Debug and release builds are then both signed with it. If `signing.properties` exists
  it wins and this shared key is ignored.
- Our key differs from upstream's, so an upstream-signed install of the same package
  must be uninstalled first. Check with `make android-verify`.

## Global rules

- Settings > Global Rules edits a YAML source stored in the core home as `rules.yaml`.
  `rules` and `prepend-rules` are placed before a profile's rules, `append-rules` after.
  Other keys (for example `profile`) are ignored, so a Clash Verge merge template can be
  pasted unchanged. Unknown keys are ignored silently, so a typo in a key name is not an error.
- Rules naming an outbound the profile does not define are skipped with a warning
  (`Skip rule [...]: target not found in profile`) instead of failing the profile.
- Validation is syntactic only (rule type and `TYPE,PAYLOAD,TARGET` shape). It does not
  load geo data, so a missing `GEOSITE` category is only caught when the profile loads.
- `profile.store-selected` is always forced off by `config/process.go`; group selections
  are stored by the app in its own database.
- The parser is `native/rulemerge`, free of mihomo dependencies; its tests run on the host
  with `make go-test`.

## Delay display

- Delays are cached per proxy name (newest result wins, whatever test URL measured it) in
  `native/delaycache`, and saved to `delays.json` in the core home. The file (version 2) keeps
  one section per profile, keyed by the profile directory path, so switching A -> B -> A, an
  app restart, or stopping/starting the service all bring a profile's delays back. Delays
  stay until the next test of that proxy replaces them.
- Switching profile writes the profile being left immediately; other saves are debounced by
  2 s and `reset()` flushes when the service stops. At most 32 profiles are kept (least
  recently used dropped), and `PruneDelays` (run after each config load) drops profiles whose
  directory no longer exists, i.e. deleted ones. The active profile is never pruned.
- Version 1 files (a single profile) are still read and are rewritten as version 2 on the
  next save.
- A failed test (`65535`) never replaces a delay that was actually measured, so a flaky or
  offline test keeps showing the last good value instead of blanking the card.
- While a test runs, proxies that have no result newer than the test start report
  `testing=true` and the card shows `…`. `ProxyActivity` re-queries the tested group every
  300 ms, so each result appears as soon as it lands instead of after the whole test.
  `finishUrlTesting` (in a `finally`) stops the progress spinner even if the test throws.
- Never-tested proxies show nothing. `GLOBAL` is tested proxy by proxy (see
  `testGroupProxies`) because its implicit provider has no test URL.
- The cache logic is `native/delaycache`, free of mihomo dependencies; its tests run on the
  host with `make go-test`.

## Connections page

- Main page > Connections (shown only while the core runs) lists every connection since the
  app process started, closed ones included: target, matched rule `TYPE(PAYLOAD)` → outbound,
  protocol, app, bytes up/down. Closed connections are dimmed and tagged "Closed". It refreshes
  every 2 seconds. Use it to confirm which rule a connection hit and which proxy carries it;
  it complements the `match ... using ...` lines in the logs.
- The core only lists live connections, so `tunnel.StartConnectionRecorder` (started from
  `coreInit`) feeds a `connview.History` from snapshots of the live set. A connection missing
  from the newest snapshot is marked closed and keeps its last seen traffic. The history holds
  1000 connections; only closed ones are evicted, oldest first. It is cleared by `reset()`
  (service stop/start), not by leaving the page.
- Sampling is every 250 ms and also right after the core logs an opened connection
  (`[TCP]/[UDP] ... --> ...` at INFO), through `native/sampler`. Polling alone missed
  connections shorter than the tick (31 listed vs 41 logged); the log trigger fixed it.
  Sub-20 ms connections can still be missed, and the trigger depends on the core's INFO log
  line format, so recheck it after every upstream sync.
- The filter box matches host, IP, rule, proxy chain, app and protocol, ignoring case.
  "Active only" hides closed ones; "Clear closed" forgets them. The toolbar sort icon cycles
  Newest / Download / Upload; the other icon closes all live connections. Long press an item
  to close it (only live ones can be closed).
- At most 500 entries are returned, and they cross Binder in slices (`ConnectionList`),
  like `ProviderList`, to stay under the 1 MB transaction limit.
- The listed rule is the one the core stored when the connection was created. A rule change
  does not affect existing connections; close them to see the new rule applied.
- The filtering, sorting and history logic is `native/connview`, and the sampling loop is
  `native/sampler`; both are free of mihomo dependencies and tested on the host with
  `make go-test`.

## Testing on an emulator

- Use a read-only AVD (`emulator -avd <name> -read-only -no-snapshot`) so the user's
  installed apps and data are untouched. Do not uninstall apps from a user's AVD.
- Serve a test profile from the host and import it with
  `adb shell am start -a android.intent.action.VIEW -d "clash://install-config?url=<url-encoded>&name=<n>"`.
  The emulator reaches the host at `10.0.2.2`.
- Check the foreground page with `adb shell dumpsys activity activities | grep mResumedActivity`
  before every tap, and find tap targets with `uiautomator dump`. Blind taps landed on
  system apps more than once. Proxy cards are custom views and expose no text to
  `uiautomator`, so read delays from a screenshot.
- Wait for a page to finish loading before tapping Save, otherwise the tap is ignored.
- `adb shell input text` turns `\n` into spaces. Type each line, then send `KEYCODE_ENTER`.
- The VPN consent dialog appears on first start; accept it only on the throwaway AVD.
- Delay tests need a reachable test URL in the profile's `proxy-groups[].url`; point it at
  the host's test server and count its access log to confirm a test really ran. To check the
  Connections page, stay on the main page while making connections (for example
  `echo -e 'GET / HTTP/1.0\r\nHost: x\r\n\r\n' | toybox nc -w 1 <host> 80` in `adb shell`), then compare the
  page's total with the number of `[TCP] ... --> ... match/using` lines in logcat. To watch
  progress, make the test server answer after a different delay per request (for example
  0.3 s + 0.9 s * n) and take screenshots about once a second after tapping the test button.
- To check delays survive a restart: `adb shell am force-stop <pkg>`, stop the test server so
  the automatic test on start fails, restart the service and open the proxy page without
  testing. `adb shell run-as <pkg> cat files/clash/delays.json` shows what was saved.
- Never run two `git` commands in parallel in this repo. A second one hits `index.lock`
  and the commits end up merged or lost.

## Git

- Commit with `git commit -S -s` (SSH signature + Signed-off-by) using conventional
  commit messages (`feat:`, `fix:`, `build:` ...). Never skip hooks or signing.
- Ask for approval before committing or pushing unless the user has already approved it.
- Push to `origin` only; never force-push `main`.
