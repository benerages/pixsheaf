#!/usr/bin/env bash
# Decides whether main gets a patch release and, if so, tags it and starts
# the release workflow (.github/workflows/patch-release.yml). Releases only
# when main has changed since the last tag, none of those commits is a
# feature or a breaking change (those need a person to choose the version),
# and every check on the head commit has passed.
#
# Needs a checkout of main with full history and tags, GH_TOKEN with
# contents:write and actions:write, and REPO=owner/name. DRY_RUN=1 prints
# the decision without tagging.
set -Eeuo pipefail
trap 'echo "patch-release: failed at line $LINENO" >&2' ERR

: "${REPO:?REPO=owner/name is required}"
# The check run of this workflow itself, which is still in progress.
self_check=${SELF_CHECK:-Tag a patch release}

note() {
  echo "$*"
  if [[ -n ${GITHUB_STEP_SUMMARY:-} ]]; then
    echo "$*" >>"$GITHUB_STEP_SUMMARY"
  fi
}

sha=$(git rev-parse HEAD)
if ! last=$(git describe --tags --abbrev=0 --match 'v[0-9]*.[0-9]*.[0-9]*' HEAD 2>/dev/null); then
  note "No release tag yet; the first release is tagged by hand."
  exit 0
fi
if [[ ! $last =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
  note "Last tag $last is not vX.Y.Z; nothing done."
  exit 0
fi
next="v${BASH_REMATCH[1]}.${BASH_REMATCH[2]}.$((BASH_REMATCH[3] + 1))"

count=$(git rev-list --count "$last..HEAD")
if [[ $count -eq 0 ]]; then
  note "No changes since $last; no release."
  exit 0
fi

subjects=$(git log --format=%s "$last..HEAD")
if git log --format=%B "$last..HEAD" | grep -q '^BREAKING CHANGE' ||
  grep -Eq '^[a-z]+(\([^)]*\))?!:' <<<"$subjects"; then
  note "Breaking change since $last; tag the next major release by hand."
  exit 0
fi
if grep -Eq '^feat(\([^)]*\))?:' <<<"$subjects"; then
  note "New features since $last; tag the next minor release by hand (patch releases contain fixes and updates only)."
  exit 0
fi

# Every check run on the head commit must have finished without failure,
# and there must be at least one besides this workflow's own.
runs=$(gh api --paginate "repos/$REPO/commits/$sha/check-runs?per_page=100" \
  -q '.check_runs[] | [.name, .status, (.conclusion // "")] | @tsv')
ok=0
bad=""
while IFS=$'\t' read -r name status conclusion; do
  [[ -z $name || $name == "$self_check" ]] && continue
  if [[ $status != completed ]]; then
    bad+="  $name: $status"$'\n'
  elif [[ $conclusion =~ ^(success|skipped|neutral)$ ]]; then
    ok=$((ok + 1))
  else
    bad+="  $name: $conclusion"$'\n'
  fi
done <<<"$runs"
statuses=$(gh api "repos/$REPO/commits/$sha/status" -q '"\(.total_count) \(.state)"')
if [[ ${statuses%% *} -gt 0 && ${statuses#* } != success ]]; then
  bad+="  commit status: ${statuses#* }"$'\n'
fi
if [[ -n $bad || $ok -eq 0 ]]; then
  note "Checks on ${sha:0:7} are not all green; no release."
  [[ -n $bad ]] && note "$bad"
  [[ $ok -eq 0 ]] && note "  no finished checks found"
  exit 0
fi

note "Releasing $next: $count commit(s) since $last, $ok check(s) green on ${sha:0:7}."
note "$(sed 's/^/  /' <<<"$subjects")"
if [[ ${DRY_RUN:-0} == 1 ]]; then
  note "DRY_RUN=1: not tagged."
  exit 0
fi

# An annotated tag through the API: no git credentials in the runner.
tag_sha=$(gh api "repos/$REPO/git/tags" -f tag="$next" -f message="$next" \
  -f object="$sha" -f type=commit -q .sha)
gh api "repos/$REPO/git/refs" -f ref="refs/tags/$next" -f sha="$tag_sha" >/dev/null
# A tag created with GITHUB_TOKEN does not trigger workflows; release.yml
# treats a manual run on a tag as a real release.
gh workflow run release.yml --repo "$REPO" --ref "$next"
note "Tagged $next and started the release workflow."
