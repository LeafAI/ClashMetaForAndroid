#!/usr/bin/env bash
# Merge the upstream project into a review branch.
#
# main is never touched: the merge happens on sync/upstream-<date>, the Go
# checks run there, and fast-forwarding main afterwards is left to the user.
set -euo pipefail

UPSTREAM_URL="${UPSTREAM_URL:-https://github.com/MetaCubeX/ClashMetaForAndroid.git}"
UPSTREAM_BRANCH="${UPSTREAM_BRANCH:-main}"
UPSTREAM_REF="${UPSTREAM_REF:-upstream/${UPSTREAM_BRANCH}}"
BASE_BRANCH="${BASE_BRANCH:-main}"

fail() {
	echo "error: $*" >&2
	exit 1
}

cd "$(git rev-parse --show-toplevel)"

[ -z "$(git status --porcelain)" ] || fail "working tree is not clean; commit or stash first"
[ "$(git rev-parse --abbrev-ref HEAD)" = "${BASE_BRANCH}" ] || fail "run this from ${BASE_BRANCH}"

git remote get-url upstream >/dev/null 2>&1 || git remote add upstream "${UPSTREAM_URL}"

# Merge conflicts in files such as go.mod tend to repeat; remember how they were resolved.
git config rerere.enabled true

# The core submodule is pinned by upstream, so there is nothing to fetch for it here.
if [ "${UPSTREAM_REF}" = "upstream/${UPSTREAM_BRANCH}" ]; then
	git fetch upstream --no-tags --no-recurse-submodules "${UPSTREAM_BRANCH}"
fi

incoming="$(git rev-list --count "${BASE_BRANCH}..${UPSTREAM_REF}")"
if [ "${incoming}" = "0" ]; then
	echo "Already up to date with ${UPSTREAM_REF}."
	exit 0
fi

echo "== ${incoming} new upstream commit(s):"
git log --oneline "${BASE_BRANCH}..${UPSTREAM_REF}"

base="$(git merge-base "${BASE_BRANCH}" "${UPSTREAM_REF}")"
overlap="$(comm -12 \
	<(git diff --name-only "${base}" "${UPSTREAM_REF}" | sort) \
	<(git diff --name-only "${base}" "${BASE_BRANCH}" | sort))"
if [ -n "${overlap}" ]; then
	echo
	echo "== files changed on both sides (watch for conflicts):"
	echo "${overlap}"
fi

branch="sync/upstream-$(date +%Y%m%d)"
git show-ref --verify --quiet "refs/heads/${branch}" && fail "branch ${branch} already exists; delete or reuse it"

git switch -c "${branch}"

if ! git merge --no-ff --no-edit --signoff "${UPSTREAM_REF}"; then
	echo
	echo "Merge stopped with conflicts on ${branch}:"
	git diff --name-only --diff-filter=U
	echo
	echo "go.mod / go.sum: keep upstream's side, then run 'go mod tidy' in core/src/main/golang."
	echo "Resolve, 'git add' the files, then 'git merge --continue' and re-run: make sync-check"
	exit 1
fi

make sync-check

echo
echo "Merged cleanly on ${branch}. Review, build a device APK, then:"
echo "  git switch ${BASE_BRANCH} && git merge --ff-only ${branch} && git push origin ${BASE_BRANCH}"
