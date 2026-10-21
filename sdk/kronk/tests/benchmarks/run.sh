#!/usr/bin/env bash
set -euo pipefail

# Invoked by make benchmark-inference. Keep the native command and its summary
# together so a failed/skipped/partial run cannot silently become current data.
selection=${1:?benchmark selection required}
benchtime=${2:?benchmark time required}
samples=${3:?sample count required}
timeout=${4:?timeout required}
[[ $samples =~ ^[1-9][0-9]*$ ]] || { echo 'sample count must be a positive integer' >&2; exit 1; }

directory=$(cd -- "$(dirname -- "$0")" && pwd)
root=$(git rev-parse --show-toplevel)
mkdir -p "$directory/results"
stamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)
raw=$(mktemp "$directory/results/$(date -u +%Y%m%dT%H%M%SZ).XXXXXX")
mv "$raw" "$raw.txt"
raw=$raw.txt
summary=$(mktemp "$directory/results/current.XXXXXX")
trap 'rm -f "$summary"' EXIT

revision=$(git rev-parse HEAD)
dirty=clean
if [[ -n $(git status --porcelain) ]]; then
    dirty=dirty
fi
{
    printf '# Current inference benchmark measurements\n\n'
    printf 'Generated from the latest successful selected run; not an approved regression baseline.\n\n'
    printf -- '- UTC: %s\n- Revision: `%s` (%s worktree)\n' "$stamp" "$revision" "$dirty"
    printf -- '- Go: `%s`\n- Host: `%s`\n' "$(go version)" "$(uname -sm)"
    printf -- '- Selection: `%s`\n- Requests/sample: `%s`; samples/case: %s; timeout: %s; GOMAXPROCS: 1\n' "$selection" "$benchtime" "$samples" "$timeout"
    printf -- '- Raw output: [results/%s](results/%s)\n\n' "$(basename "$raw")" "$(basename "$raw")"
    printf 'Native library/model hashes, device, drivers, power state and offload/thread settings must also match for a valid comparison. See README.md.\n\n'
} > "$summary"
cat "$summary" > "$raw"
printf '\n--- benchmark output ---\n' >> "$raw"

echo "Saving benchmark output to $raw"
if ! GITHUB_WORKSPACE="$root" KRONK_BENCHMARK=true \
    go -C "$root" test ./sdk/kronk/tests/benchmarks -run '^$' \
    -bench "$selection" -benchtime="$benchtime" -count="$samples" \
    -timeout="$timeout" -cpu=1 -v 2>&1 | tee -a "$raw"; then
    echo "Benchmark failed; CURRENT.md unchanged. Raw output retained: $raw" >&2
    exit 1
fi

full=0
[[ $selection != '^BenchmarkInferenceMatrix$' ]] || full=1
if ! awk -v samples="$samples" -v full="$full" '
function median(key, metric,    a,n,i,j,v) {
    split("", a)
    n = count[key]
    for (i=1; i<=n; i++) {
        v = values[key,metric,i]
        j = i-1
        while (j>0 && a[j]>v) { a[j+1]=a[j]; j-- }
        a[j+1]=v
    }
    return n%2 ? a[(n+1)/2] : (a[n/2]+a[n/2+1])/2
}
/^--- benchmark output ---$/ { output=1; next }
output && /^BenchmarkInferenceMatrix\// && $2 ~ /^[0-9]+$/ {
    key=$1
    if (!(key in count)) order[++cases]=key
    n=++count[key]
    for (i=3; i<NF; i+=2) {
        if ($i !~ /^[0-9]+([.][0-9]*)?([eE][+-]?[0-9]+)?$/) bad=1
        values[key,$(i+1),n]=$i+0
        seen[key,$(i+1),n]=1
    }
    for (i=1; i<=3; i++) {
        metric=(i==1 ? "ns/op" : i==2 ? "ttft-ms/op" : "decode-tok/s")
        if (!seen[key,metric,n] || values[key,metric,n]<=0) bad=1
    }
    if (!seen[key,"prompt-tok/op",n] || values[key,"prompt-tok/op",n]<=0 ||
        !seen[key,"cached-tok/op",n] || !seen[key,"output-tok/op",n] ||
        values[key,"output-tok/op",n]!=256) bad=1
}
END {
    if (!cases || bad || (full && cases!=24)) exit 1
    for (i=1; i<=cases; i++) if (count[order[i]]!=samples) exit 1
    printf "Measured cases: **%d** (%s). Values below are medians across %d samples, not significance tests.\n\n", cases, full ? "full selected matrix" : "filtered selection; not full-matrix coverage", samples
    print "| Case | Request ms/op | TTFT ms/op | Decode tokens/s | Prompt tokens/op | Cached tokens/op | Output tokens/op | Draft acceptance |"
    print "| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |"
    for (i=1; i<=cases; i++) {
        key=order[i]
        label=key
        sub(/^BenchmarkInferenceMatrix\//,"",label)
        acceptance="—"
        if (seen[key,"draft-acceptance/op",1]) acceptance=sprintf("%.4f",median(key,"draft-acceptance/op"))
        printf "| `%s` | %.3f | %.3f | %.3f | %.1f | %.1f | %.1f | %s |\n", label, median(key,"ns/op")/1000000, median(key,"ttft-ms/op"), median(key,"decode-tok/s"), median(key,"prompt-tok/op"), median(key,"cached-tok/op"), median(key,"output-tok/op"), acceptance
    }
}' "$raw" >> "$summary"; then
    echo "Missing/invalid benchmark samples; CURRENT.md unchanged. Raw output retained: $raw" >&2
    exit 1
fi

mv "$summary" "$directory/CURRENT.md"
echo "Updated $directory/CURRENT.md"
