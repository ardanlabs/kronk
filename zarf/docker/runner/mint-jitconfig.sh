#!/usr/bin/env bash
#
# Print a single-use JIT runner config for one ephemeral runner.
#
#   NAME=kronk-linux-gpu-1 LABELS=gpu,vulkan APP_ID=... APP_KEY=~/key.pem \
#       ./mint-jitconfig.sh
#
# Runs on the HOST, never in a container: the App private key is the org's
# long-lived credential, and a job that can read it can register runners into
# the org. What the container gets instead is the output of this script — a
# config for one named runner, good for one job, already consumed by the time a
# job starts (start-runners.sh mints a fresh one per job).
#
#   NAME      runner name, must be unique in the org  (required)
#   LABELS    comma-separated runner labels           (required)
#   APP_ID    GitHub App id                           (required)
#   APP_KEY   path to the App private key .pem        (required)
#   ORG       GitHub org                              (default ardanlabs)
#   GROUP     runner group                            (default kronk)
#   WORK      runner work folder inside the container (default /_work)
#
# The App needs organization_self_hosted_runners: write, the same permission
# the registration-token flow it replaces used.

set -euo pipefail

NAME="${NAME:?NAME is required}"
LABELS="${LABELS:?LABELS is required}"
APP_ID="${APP_ID:?APP_ID is required}"
APP_KEY="${APP_KEY:?APP_KEY is required}"
ORG="${ORG:-ardanlabs}"
GROUP="${GROUP:-kronk}"
WORK="${WORK:-/_work}"

if [[ ! -r "$APP_KEY" ]]; then
    echo "cannot read APP_KEY: $APP_KEY" >&2
    exit 1
fi

api() { curl -fsSL -H "Accept: application/vnd.github+json" "$@"; }

b64url() { openssl base64 -A | tr '+/' '-_' | tr -d '='; }

# App JWTs are capped at 10 minutes and rejected if `iat` is even slightly in
# the future, so this claims 9 with a minute of backdating for clock skew.
now="$(date +%s)"
claims="$(printf '{"alg":"RS256","typ":"JWT"}' | b64url).$(
    printf '{"iat":%d,"exp":%d,"iss":"%s"}' "$((now - 60))" "$((now + 540))" "$APP_ID" | b64url)"
jwt="${claims}.$(printf '%s' "$claims" | openssl dgst -sha256 -sign "$APP_KEY" -binary | b64url)"

# A JWT authenticates the App itself, which can do nothing to an org until it
# trades itself for a token scoped to the org's installation.
install_id="$(api -H "Authorization: Bearer $jwt" \
    "https://api.github.com/orgs/$ORG/installation" | jq -r .id)"
token="$(api -X POST -H "Authorization: Bearer $jwt" \
    "https://api.github.com/app/installations/$install_id/access_tokens" | jq -r .token)"

# By name, not a hardcoded id: a group recreated in the org UI keeps its name
# and gets a new id. Registering into the wrong group leaves every job queued
# with no error, because kronk is public and Default excludes public repos.
group_id="$(api -H "Authorization: Bearer $token" \
    "https://api.github.com/orgs/$ORG/actions/runner-groups" |
    jq -r --arg g "$GROUP" '.runner_groups[] | select(.name == $g) | .id')"
if [[ -z "$group_id" ]]; then
    echo "no runner group named '$GROUP' in org '$ORG'" >&2
    exit 1
fi

api -X POST -H "Authorization: Bearer $token" \
    "https://api.github.com/orgs/$ORG/actions/runners/generate-jitconfig" \
    -d "$(jq -n --arg n "$NAME" --arg l "$LABELS" --argjson g "$group_id" --arg w "$WORK" \
        '{name: $n, runner_group_id: $g, labels: ($l | split(",")), work_folder: $w}')" |
    jq -r .encoded_jit_config
