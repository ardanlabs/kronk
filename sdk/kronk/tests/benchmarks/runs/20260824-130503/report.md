# Code-generation benchmark report

Run: `20260824-130503`  
Generated: 2026-08-24T14:11:41-04:00  
Models: 4

## Summary

| Model | Attempts | Score | Buildable | Fully passed |
| --- | ---: | ---: | ---: | ---: |
| Gemma4-26B-A4B-Q8 | 3 | 80.00% | 100.00% | 33.33% |
| MTP-Qwen36-35B-A3B-Q8 | 3 | 55.00% | 0.00% | 0.00% |
| Ornith15-35B-Q8 | 3 | 5.00% | 0.00% | 0.00% |
| Qwen38-27B-Q4 | 3 | 0.00% | 0.00% | 0.00% |

Buildable is the percentage of attempts that passed `go build`. Fully passed is the percentage that passed every structural, build, vet, unit, and end-to-end scenario check.

## Performance

| Model | tok/s | TTFT | Wall/op | Prompt tok/op | Completion tok/op | Reasoning tok/op | Draft accept | Draft coverage |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Gemma4-26B-A4B-Q8 | 108.00 | 193 ms | 137.99 s | 1518 | 14882 | 10809 | 79.84% | 99.99% |
| MTP-Qwen36-35B-A3B-Q8 | 99.89 | 191 ms | 115.73 s | 1491 | 11541 | 10397 | 78.22% | 99.99% |
| Ornith15-35B-Q8 | 63.80 | 184 ms | 257.78 s | 1491 | 16384 | 15580 | 33.93% | 99.99% |
| Qwen38-27B-Q4 | 22.86 | 808 ms | 717.64 s | 1529 | 16384 | 16384 | 55.22% | 99.99% |

Performance metrics are averages across attempts.

## Resources

| Model | Model | Slot | Total memory | SDK heap/op |
| --- | ---: | ---: | ---: | ---: |
| Gemma4-26B-A4B-Q8 | 25.72 GiB | 28160 MiB | 53.70 GiB | -606.62 MiB |
| MTP-Qwen36-35B-A3B-Q8 | 36.40 GiB | 2811 MiB | 39.57 GiB | 26.83 MiB |
| Ornith15-35B-Q8 | 35.20 GiB | 5371 MiB | 40.86 GiB | 26.82 MiB |
| Qwen38-27B-Q4 | 16.34 GiB | 8790 MiB | 25.56 GiB | 86.22 MiB |

SDK heap is Go-managed memory only. Model, slot, and total-memory estimates include native llama.cpp allocations.

## Configuration

| Model | Model ID | Type | Quant | Context | Sampling | Max tokens | Thinking | Speculation | llama.cpp |
| --- | --- | --- | --- | ---: | --- | ---: | --- | --- | --- |
| Gemma4-26B-A4B-Q8 | `unsloth/gemma-4-26B-A4B-it-UD-Q8_K_XL/AGENT` | moe | Q8_0 | 131072 | temp=1, top-k=64, top-p=0.95 | 16384 | true | auto | b10608 |
| MTP-Qwen36-35B-A3B-Q8 | `unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT` | hybrid | Q8_0 | 131072 | temp=1, top-k=20, top-p=0.95 | 16384 | true | auto | b10608 |
| Ornith15-35B-Q8 | `ornith-ai/Ornith-1.5-35B-Q8_0/AGENT` | hybrid | Q8_0 | 262144 | temp=1, top-k=20, top-p=0.95 | 16384 | true | auto | b10608 |
| Qwen38-27B-Q4 | `unsloth/Qwen3.8-27B-UD-Q4_K_XL/AGENT` | hybrid | Q4_K - Medium | 131072 | temp=1, top-k=20, top-p=0.95 | 16384 | true | auto | b10608 |

## Iterations

### Gemma4-26B-A4B-Q8

#### Iteration 1

- Score: 20/20 (100.00%), finish `stop`

#### Iteration 2

- Score: 14/20 (70.00%), finish `length`

**Failure — `scenario-initial-board`**

```text
run failed: context deadline exceeded; output: 
Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):
Score: X: 0 | O: 0 | Draws: 0

 X | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9


Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):
Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 X | 5 | 6
-----------
 7 | 8 | 9


Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):
Score: X: 0 | O: 0 | Draws: 0

 1 |...
```

**Failure — `scenario-x-win`**

```text
run failed: context deadline exceeded; output: 
Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):
Score: X: 0 | O: 0 | Draws: 0

 X | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9


Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):
Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 X | 5 | 6
-----------
 7 | 8 | 9


Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):
Score: X: 0 | O: 0 | Draws: 0

 1 |...
```

**Failure — `scenario-o-win`**

```text
run failed: context deadline exceeded; output: 
Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):
Score: X: 0 | O: 0 | Draws: 0

 X | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9


Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):
Score: X: 0 | O: 0 | Draws: 0

 1 | X | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9


Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):
Score: X: 0 | O: 0 | Draws: 0

 1 |...
```

**Failure — `scenario-draw`**

```text
run failed: context deadline exceeded; output: 
Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):
Score: X: 0 | O: 0 | Draws: 0

 X | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9


Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):
Score: X: 0 | O: 0 | Draws: 0

 1 | X | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9


Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):
Score: X: 0 | O: 0 | Draws: 0

 1 |...
```

**Failure — `scenario-invalid-input`**

```text
run failed: context deadline exceeded; output: 
Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):Invalid move. Enter an empty position from 1 to 9.
Player X's turn. Enter a number (1-9):Invalid move. Enter an empty position from 1 to 9.
Player X's turn. Enter a number (1-9):
Score: X: 0 | O: 0 | Draws: 0

 X | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9


Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):
Score: X: 0 | O: 0 | Draws: 0

 X | 2 | 3
-----------
 4 | 5 | 6
----------...
```

**Failure — `scenario-replay-score`**

```text
run failed: context deadline exceeded; output: 
Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):
Score: X: 0 | O: 0 | Draws: 0

 X | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9


Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):
Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 X | 5 | 6
-----------
 7 | 8 | 9


Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):
Score: X: 0 | O: 0 | Draws: 0

 1 |...
```

#### Iteration 3

- Score: 14/20 (70.00%), finish `length`

**Failure — `scenario-initial-board`**

```text
run failed: context deadline exceeded; output: 
Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):
Score: X: 0 | O: 0 | Draws: 0

 X | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9


Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):
Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 X | 5 | 6
-----------
 7 | 8 | 9


Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):
Score: X: 0 | O: 0 | Draws: 0

 1 |...
```

**Failure — `scenario-x-win`**

```text
run failed: context deadline exceeded; output: 
Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):
Score: X: 0 | O: 0 | Draws: 0

 X | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9


Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):
Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 X | 5 | 6
-----------
 7 | 8 | 9


Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):
Score: X: 0 | O: 0 | Draws: 0

 1 |...
```

**Failure — `scenario-o-win`**

```text
run failed: context deadline exceeded; output: 
Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):
Score: X: 0 | O: 0 | Draws: 0

 X | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9


Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):
Score: X: 0 | O: 0 | Draws: 0

 1 | X | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9


Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):
Score: X: 0 | O: 0 | Draws: 0

 1 |...
```

**Failure — `scenario-draw`**

```text
run failed: context deadline exceeded; output: 
Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):
Score: X: 0 | O: 0 | Draws: 0

 X | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9


Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):
Score: X: 0 | O: 0 | Draws: 0

 1 | X | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9


Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):
Score: X: 0 | O: 0 | Draws: 0

 1 |...
```

**Failure — `scenario-invalid-input`**

```text
run failed: context deadline exceeded; output: 
Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):Invalid move. Enter an empty position from 1 to 9.
Player X's turn. Enter a number (1-9):Invalid move. Enter an empty position from 1 to 9.
Player X's turn. Enter a number (1-9):
Score: X: 0 | O: 0 | Draws: 0

 X | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9


Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):
Score: X: 0 | O: 0 | Draws: 0

 X | 2 | 3
-----------
 4 | 5 | 6
----------...
```

**Failure — `scenario-replay-score`**

```text
run failed: context deadline exceeded; output: 
Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):
Score: X: 0 | O: 0 | Draws: 0

 X | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9


Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):
Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 X | 5 | 6
-----------
 7 | 8 | 9


Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):
Score: X: 0 | O: 0 | Draws: 0

 1 |...
```

### MTP-Qwen36-35B-A3B-Q8

#### Iteration 1

- Score: 11/20 (55.00%), finish `stop`

**Failure — `go-build`**

```text
# tictactoe
./main.go:98:13: cannot use cur (variable of type rune) as byte value in assignment
```

#### Iteration 2

- Score: 11/20 (55.00%), finish `stop`

**Failure — `go-build`**

```text
# tictactoe
./main.go:98:13: cannot use cur (variable of type rune) as byte value in assignment
```

#### Iteration 3

- Score: 11/20 (55.00%), finish `stop`

**Failure — `go-build`**

```text
# tictactoe
./main.go:98:13: cannot use cur (variable of type rune) as byte value in assignment
```

### Ornith15-35B-Q8

#### Iteration 1

- Score: 1/20 (5.00%), finish `length`

**Failure — `go-parse`**

```text
main.go:103:2: expected 1 expression (and 4 more errors)
```

#### Iteration 2

- Score: 1/20 (5.00%), finish `length`

**Failure — `go-parse`**

```text
main.go:103:2: expected 1 expression (and 4 more errors)
```

#### Iteration 3

- Score: 1/20 (5.00%), finish `length`

**Failure — `go-parse`**

```text
main.go:103:2: expected 1 expression (and 4 more errors)
```

### Qwen38-27B-Q4

#### Iteration 1

- Score: 0/20 (0.00%), finish `length`

**Failure — `source-extracted`**

```text
no complete package main source found
```

#### Iteration 2

- Score: 0/20 (0.00%), finish `length`

**Failure — `source-extracted`**

```text
no complete package main source found
```

#### Iteration 3

- Score: 0/20 (0.00%), finish `length`

**Failure — `source-extracted`**

```text
no complete package main source found
```
