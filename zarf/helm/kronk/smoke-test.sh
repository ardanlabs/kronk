#!/usr/bin/env bash
# Installs only into a new namespace on an explicitly named, disposable kind
# context. Deletes that namespace and its test data on exit, never the cluster.
set -euo pipefail
if [[ $# != 1 || "$1" != kind-* ]]; then
  echo "Usage: $0 kind-<disposable-cluster-name>" >&2
  exit 2
fi
context=$1
chart=$(cd "$(dirname "$0")" && pwd)
namespace="kronk-smoke-$(date +%s)-$$"
k=(kubectl --context "$context" -n "$namespace")
h=(helm --kube-context "$context" -n "$namespace")
keydir=""
cleanup() {
  code=$?
  if (( code != 0 )); then
    "${k[@]}" get pods,pvc >&2 || true
    "${k[@]}" describe pod kronk-0 >&2 || true
    "${k[@]}" logs kronk-0 --all-containers --tail=100 >&2 || true
  fi
  "${h[@]}" uninstall kronk --wait --timeout 2m >/dev/null 2>&1 || true
  "${k[@]}" delete namespace "$namespace" --wait=false >/dev/null || true
  if [[ -n "$keydir" ]]; then rm -rf "$keydir"; fi
  exit "$code"
}
kubectl --context "$context" create namespace "$namespace"
trap cleanup EXIT
# Admission checks all application and Helm test containers under restricted PSS.
"${k[@]}" label namespace "$namespace" pod-security.kubernetes.io/enforce=restricted
"${h[@]}" install kronk "$chart" -f "$chart/examples/cpu.yaml" --wait --timeout 20m
"${h[@]}" test kronk --timeout 2m

chat() {
  # Token never appears in host arguments or logs. curl runs inside the pod.
  "${k[@]}" exec kronk-0 -c kronk -- /bin/sh -ec '
    token=$(cat /kronk/keys/master.jwt)
    curl -fsS --max-time 300 http://kronk:11435/v1/chat/completions \
      -H "Authorization: Bearer $token" -H "Content-Type: application/json" \
      -d '\''{"model":"unsloth/Qwen3-0.6B-Q8_0","messages":[{"role":"user","content":"Say hello."}],"max_tokens":256,"temperature":0,"chat_template_kwargs":{"enable_thinking":false}}'\''
  ' | jq -e '.choices[0].message.content | type == "string" and length > 0' >/dev/null
}
chat
"${k[@]}" exec kronk-0 -c kronk -- /bin/sh -ec '
  code=$(curl -sS -o /dev/null -w "%{http_code}" http://kronk:11435/v1/models)
  test "$code" = 401
  printf retained > /kronk/smoke-retained
'
"${k[@]}" delete pod kronk-0 --wait=true
"${k[@]}" rollout status statefulset/kronk --timeout=20m
"${k[@]}" exec kronk-0 -c kronk -- /bin/sh -ec 'test "$(cat /kronk/smoke-retained)" = retained'
chat
# Change a real setting to exercise rollout, then restore the initial revision.
"${h[@]}" upgrade kronk "$chart" -f "$chart/examples/cpu.yaml" \
  --set env.KRONK_POOL_BUDGET_PERCENT=80 --wait --timeout 20m
chat
"${h[@]}" rollback kronk 1 --wait --timeout 20m
chat
"${h[@]}" test kronk --timeout 2m

# Preserve the existing keyset in a Secret, then prove removing its non-master
# signing key invalidates a previously accepted token after a restart.
keydir=$(mktemp -d)
mkdir "$keydir/keys"
"${k[@]}" exec kronk-0 -c kronk -- tar -C /kronk/keys -cf - . | tar -xf - -C "$keydir/keys"
"${k[@]}" exec kronk-0 -c kronk -- /bin/sh -ec '
  token=$(cat /kronk/keys/master.jwt)
  curl -fsS http://kronk:11435/v1/security/token/create \
    -H "Authorization: Bearer $token" -H "Content-Type: application/json" \
    -d '\''{"admin":false,"duration":3600000000000}'\''
' | jq -er .token > "$keydir/token.jwt"
"${k[@]}" create secret generic smoke-keys --from-file="$keydir/keys/" --dry-run=client -o yaml | "${k[@]}" apply -f -
"${h[@]}" upgrade kronk "$chart" -f "$chart/examples/cpu.yaml" \
  --set auth.existingSecret=smoke-keys --wait --timeout 20m
token_status() {
  "${k[@]}" exec -i kronk-0 -c kronk -- /bin/sh -ec '
    token=$(cat)
    curl -sS -o /dev/null -w "%{http_code}" http://kronk:11435/v1/models \
      -H "Authorization: Bearer $token"
  ' < "$keydir/token.jwt"
}
test "$(token_status)" = 200
find "$keydir/keys" -name '*.pem' ! -name master.pem -delete
"${k[@]}" create secret generic smoke-keys --from-file="$keydir/keys/" --dry-run=client -o yaml | "${k[@]}" apply -f -
"${k[@]}" rollout restart statefulset/kronk
"${k[@]}" rollout status statefulset/kronk --timeout=20m
test "$(token_status)" = 401
chat
echo "PASS: authenticated chat, unauthenticated rejection, PVC reuse, upgrade, rollback, Helm hooks, Secret key revocation"
