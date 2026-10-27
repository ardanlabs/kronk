#!/usr/bin/env bash
set -euo pipefail
chart=$(cd "$(dirname "$0")" && pwd)
for variant in cpu cuda vulkan rocm all; do
  helm lint --strict "$chart" --set "image.variant=$variant"
done
python3 "$chart/tests/chart_test.py"
