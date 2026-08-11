#!/usr/bin/env bash
# Replay captured HTTP request bodies sequentially and save their raw responses.

set -euo pipefail

usage() {
	cat <<'EOF'
usage: replay.sh CAPTURE_DIR OUTPUT_DIR [UPSTREAM_URL]

Replays request-NNNN.json files using the method and path recorded in the
matching request-NNNN.headers.json files. Raw responses are saved as .sse files.
UPSTREAM_URL defaults to http://127.0.0.1:11435.

Set KRONK_TOKEN to add an Authorization bearer token to replayed requests.
EOF
}

if [[ ${1:-} == "-h" || ${1:-} == "--help" ]]; then
	usage
	exit 0
fi

if [[ $# -lt 2 || $# -gt 3 ]]; then
	usage >&2
	exit 2
fi

for dependency in curl jq; do
	command -v "$dependency" >/dev/null || {
		echo "missing required tool: $dependency" >&2
		exit 2
	}
done

capture_dir=$1
output_dir=$2
upstream_url=${3:-http://127.0.0.1:11435}

[[ -d $capture_dir ]] || {
	echo "capture directory does not exist: $capture_dir" >&2
	exit 2
}

if [[ -d $output_dir && -n $(find "$output_dir" -mindepth 1 -maxdepth 1 -print -quit) ]]; then
	echo "output directory is not empty: $output_dir" >&2
	exit 2
fi
mkdir -p "$output_dir"

shopt -s nullglob
requests=("$capture_dir"/request-[0-9][0-9][0-9][0-9].json)
if (( ${#requests[@]} == 0 )); then
	echo "no captured request bodies found in $capture_dir" >&2
	exit 2
fi

for request in "${requests[@]}"; do
	stem=${request%.json}
	metadata="$stem.headers.json"
	[[ -f $metadata ]] || {
		echo "missing request metadata: $metadata" >&2
		exit 2
	}

	method=$(jq -er '.method' "$metadata")
	path=$(jq -er '.path' "$metadata")
	name=$(basename "$stem")
	response="$output_dir/$name.sse"
	headers=(-H 'Content-Type: application/json')
	if [[ -n ${KRONK_TOKEN:-} ]]; then
		headers+=(-H "Authorization: Bearer $KRONK_TOKEN")
	fi

	echo "replaying $name: $method $path"
	curl --fail-with-body --silent --show-error \
		-X "$method" \
		"${upstream_url%/}$path" \
		"${headers[@]}" \
		--data-binary "@$request" \
		>"$response"
done

echo "saved ${#requests[@]} responses to $output_dir"
