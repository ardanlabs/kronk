# Quick-check release history
## v1.32.9 → v1.33.0

- Commits: `3b4b84aadb90be4e5930891d4b020d73011985bf` → `6fbb46794634c286240f783fb0646ad8064b8d3e`
- Cell: MoE / one slot / IMC on / MTP on / images off
- Comparison: **PASS**; saved runs: baseline **PASS** / candidate **PASS**
- Baseline: 2026-10-22T21:28:50.82593Z; model: `unsloth/mtp-Qwen3.6-35B-A3B-UD-Q2_K_XL/QUICK-CHECK`; workload: `operations-v2`; library: `b11513`
- Baseline machine: `Ramons-MacBook-Pro.local`; RAM: 2.5769803776e+10 bytes; VRAM: 1.906966528e+10 bytes; unified memory: true
- Candidate: 2026-10-22T22:29:35.295777Z; model: `unsloth/mtp-Qwen3.6-35B-A3B-UD-Q2_K_XL/QUICK-CHECK`; workload: `operations-v2`; library: `b11797`
- Candidate machine: `Ramons-MacBook-Pro.local`; RAM: 2.5769803776e+10 bytes; VRAM: 1.906966528e+10 bytes; unified memory: true

| Seconds | Baseline median [SD; n] | Candidate median [SD; n] | Change |
| --- | ---: | ---: | ---: |
| Warm-up request | 4.082761 [SD unavailable; n 1] | 6.178691 [SD unavailable; n 1] | +51.34% |
| Warmed workload | 41.112894 [SD 1.969372; n 3] | 40.854652 [SD 1.887656; n 3] | -0.63% |

