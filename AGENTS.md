# AGENTS.md

This is a private fork of MetaCubeX/ClashMetaForAndroid (`origin` = LeafAI fork,
`upstream` = MetaCubeX). Changes are **not** sent back upstream, so the goal is to
keep our delta small and easy to re-merge.

## Our changes on top of upstream

| Area | Where |
|---|---|
| Delay results shared across views, kept across mode switches | `core/src/main/golang/native/tunnel/delay_cache.go`, `proxies.go`, `connectivity.go`; `ProxyActivity.kt`, `ProxyDesign.kt` |
| `GLOBAL` group always listed, tested directly | `tunnel/proxies.go`, `tunnel/connectivity.go` |
| Global rules (YAML source merged into every profile) | `native/rulemerge/`, `native/config/rules.go`, `RuleSourceActivity.kt`, `RuleSourceDesign.kt`, `design_rule_source.xml` |
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
`GLOBAL` delay test, and a global rule saved in Settings > Global Rules taking effect.

The `core/src/foss/golang/clash` submodule (mihomo) is pinned by upstream. Do not bump it
ourselves. After a merge run `git submodule update --init --depth 1`.

## Build

- `make android-debug` / `make android-release` — build APKs (`FLAVOR=Meta|Alpha`, default Meta).
  Output: `app/build/outputs/apk/<flavor>/<type>/`.
- `make android-install` — build debug and `adb install` the `ABI` build (default arm64-v8a).
- `make android-verify` — print the signing certificate of built APKs.
- `make go-vet` / `make go-test` — check the native Go packages (also `make sync-check`).
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
  the host's test server and count its access log to confirm a test really ran.
- Never run two `git` commands in parallel in this repo. A second one hits `index.lock`
  and the commits end up merged or lost.

## Git

- Commit with `git commit -S -s` (SSH signature + Signed-off-by) using conventional
  commit messages (`feat:`, `fix:`, `build:` ...). Never skip hooks or signing.
- Ask for approval before committing or pushing unless the user has already approved it.
- Push to `origin` only; never force-push `main`.
