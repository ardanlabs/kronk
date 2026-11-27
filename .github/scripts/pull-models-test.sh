#!/usr/bin/env bash
# Exercise the model-list contract without network access or model downloads.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/pull-models-test.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT
export PULL_MODEL_TEST_CALLS="$tmp/calls"
export PATH="$tmp:$PATH"

cat > "$tmp/kronk" <<'EOF'
#!/usr/bin/env bash
printf '<%s>' "$@" >> "$PULL_MODEL_TEST_CALLS"
printf '\n' >> "$PULL_MODEL_TEST_CALLS"
if [[ "$*" == *fail-model* ]]; then
  exit 9
fi
EOF
chmod +x "$tmp/kronk"

cat > "$tmp/models" <<'EOF'
# Comments and blank lines must not invoke the CLI.

kronk org/target # Existing two-column entries still work.
bucky ggml-tiny.bin
  kronk org/inkling dspark https://huggingface.co/other/draft/resolve/main/Mainline_Support/head.gguf_file?download=true
EOF
# The final entry deliberately has no newline.
printf 'kronk org/mtp mtp https://huggingface.co/org/draft/resolve/main/head.gguf' >> "$tmp/models"
bash "$script_dir/pull-models.sh" "$tmp/models"
cat > "$tmp/expected" <<'EOF'
<model><pull><--local><org/target>
<bucky><model><pull><--local><ggml-tiny.bin>
<model><pull><--local><org/inkling><--draft-method><dspark><--draft><https://huggingface.co/other/draft/resolve/main/Mainline_Support/head.gguf_file?download=true>
<model><pull><--local><org/mtp><--draft-method><mtp><--draft><https://huggingface.co/org/draft/resolve/main/head.gguf>
EOF
diff -u "$tmp/expected" "$PULL_MODEL_TEST_CALLS"

for entry in \
  'kronk' \
  'kronk org/target dspark' \
  'kronk org/target dspark https://example.com/head extra' \
  'bucky ggml-tiny.bin dspark https://example.com/head' \
  'unknown org/target'; do
  : > "$PULL_MODEL_TEST_CALLS"
  printf '%s\n' "$entry" > "$tmp/models"
  if bash "$script_dir/pull-models.sh" "$tmp/models" > "$tmp/output" 2>&1; then
    echo "Expected invalid entry to fail: $entry" >&2
    exit 1
  fi
  if [[ -s "$PULL_MODEL_TEST_CALLS" ]]; then
    echo "Invalid entry invoked the CLI: $entry" >&2
    exit 1
  fi
done

: > "$PULL_MODEL_TEST_CALLS"
printf 'kronk fail-model\nkronk must-not-run\n' > "$tmp/models"
if bash "$script_dir/pull-models.sh" "$tmp/models" > "$tmp/output" 2>&1; then
  echo 'Expected download failure to propagate' >&2
  exit 1
fi
printf '<model><pull><--local><fail-model>\n' > "$tmp/expected"
diff -u "$tmp/expected" "$PULL_MODEL_TEST_CALLS"

echo 'Model-list download tests passed'
