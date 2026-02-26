# Chapter 12: Browser UI (BUI)

## Table of Contents

- [12.1 Accessing the BUI](#121-accessing-the-bui)
- [12.2 Downloading Libraries](#122-downloading-libraries)
- [12.3 Downloading Models](#123-downloading-models)
- [12.4 Managing Keys and Tokens](#124-managing-keys-and-tokens)
- [12.5 Other Screens](#125-other-screens)
- [12.6 Model Playground](#126-model-playground)
  - [12.6.1 Automated Mode](#1261-automated-mode)
  - [12.6.2 Manual Mode](#1262-manual-mode)

---



Kronk includes a web-based interface for managing models, libraries,
security, and server configuration without using the command line.

### 12.1 Accessing the BUI

The BUI is served from the same port as the API.

**Open in Browser:**

```
http://localhost:8080
```

The BUI automatically loads when you navigate to the server root.

### 12.2 Downloading Libraries

Before running inference, you need the llama.cpp libraries.

**Steps:**

1. Navigate to the **Libraries** page from the menu
2. Click **Pull Libraries**
3. Wait for the download to complete

The BUI auto-detects your platform (OS, architecture, GPU) and downloads
the appropriate binaries to `~/.kronk/libraries/`.

**Override Detection:**

If auto-detection is incorrect, you can specify:

- Processor type (CPU, CUDA, Metal, ROCm, Vulkan)
- Architecture (amd64, arm64)
- Operating system

### 12.3 Downloading Models

**Browse the Catalog:**

1. Navigate to the **Catalog** page
2. Browse available models by category:
   - Text-Generation
   - Image-Text-to-Text (Vision)
   - Audio-Text-to-Text
   - Embedding
   - Reranking
3. Click **Pull** next to a model to download it

**Monitor Progress:**

The BUI shows real-time download progress including:

- Download percentage
- Transfer speed
- Estimated time remaining

**View Pulled Models:**

Navigate to the **Models** page to see all downloaded models and their
status.

### 12.4 Managing Keys and Tokens

When authentication is enabled, use the BUI to manage security.

**Keys Page:**

- View all signing keys with their IDs and creation dates
- Create new signing keys
- Delete keys (except master key)

**Tokens Page:**

- Generate new tokens with specific:
  - Duration (hours, days)
  - Endpoint access (chat-completions, embeddings, etc.)
  - Rate limits (requests per day/month/year)
- Copy generated tokens to clipboard

**Note:** You must provide an admin token in the BUI settings to access
security management features.

### 12.5 Other Screens

**Dashboard:**

Overview of server status, loaded models, and system information.

**Documentation:**

Built-in SDK and CLI documentation accessible from the menu:

- SDK API reference
- CLI command reference
- Example code

**Settings:**

Configure BUI preferences:

- API token for authenticated requests
- Theme preferences

### 12.6 Model Playground

The Model Playground is an interactive testing environment for evaluating
models directly in the BUI. It supports two operating modes — **Automated**
and **Manual** — accessible from the sidebar.

**Steps:**

1. Navigate to the **Playground** page from the menu (or go to `/playground`)
2. Select a model from the dropdown, or choose **New…** to pull a GGUF file
   by HuggingFace URL (with optional projection URL for vision/audio models)
3. Choose a **Template Mode**:
   - **Builtin** — select a chat template from the catalog (or leave as Auto)
   - **Custom** — paste a Jinja template script
4. Configure model parameters: Context Window, NBatch, NUBatch, NSeqMax,
   Flash Attention (auto/enabled/disabled), KV Cache Type (f16/q8_0/q4_0),
   and Cache Mode (None/SPC/IMC)
5. Select **Automated Mode** or **Manual Mode**

#### 12.6.1 Automated Mode

Automated mode runs structured test suites against a model and scores the
results. It is designed for benchmarking model quality and finding optimal
configurations without manual interaction.

**Sweep Modes:**

- **Sampling Sweep** — Varies sampling parameters (temperature, top_p, top_k,
  min_p, and others including repetition, DRY/XTC controls, and reasoning
  settings) using user-defined value ranges while holding the model
  configuration fixed. Each parameter accepts comma-separated values; the first
  value is the baseline and additional values define the sweep range. When
  catalog defaults are available for the selected model, they are displayed
  next to the parameter name as a hint. Requires a loaded session.
- **Config Sweep** — Varies model configuration parameters (context window,
  nbatch, nubatch, nseq_max, flash attention, cache type, cache mode) as a
  full cross-product of user-selected values. Each candidate reloads the model
  with a new session, making it slower than sampling sweeps. Does **not**
  require a pre-loaded session.

**⚠** Unload the current session before running config sweeps.

**Scenarios:**

Two test scenarios can be enabled independently:

- **Chat Quality** — Tests text generation with math problems, translations,
  list formatting, and multi-turn conversations. Responses are scored using
  exact match (with partial credit for contained answers) and regex validation.
  Config sweeps additionally include code generation and instruction-following
  prompts for throughput measurement.
- **Tool Calling** — Tests function calling with 10 built-in tool definitions
  (`get_weather`, `add`, `search_products`, `send_email`, `get_stock_price`,
  `convert_currency`, `create_calendar_event`, `translate_text`,
  `get_directions`, `set_reminder`). Validates that the model emits tool calls
  with valid JSON arguments and required fields. Includes multi-turn tool
  calling scenarios.

If tool calling is enabled, automated mode probes the template for tool
calling compatibility before running. If the probe fails, it falls back to
chat-only tests automatically.

**Context Fill Testing:**

When chat scenarios are enabled, automated mode calibrates context fill prompts
at 20%, 50%, and 80% of the context window. These prompts fill the
conversation with background text to measure TPS degradation as the KV cache
fills. The first prompt in each scenario is used as a warmup; TPS and TTFT
averages exclude warmup results.

**Repeats:**

Each prompt can be run multiple times (configurable 1–20, default 3) with
scores averaged for more stable results.

**Running Tests:**

1. Select **Sampling Sweep** or **Config Sweep**
2. Configure the sweep value ranges (sampling) or sweep value sets (config).
   For sampling sweeps, enter comma-separated values for each parameter — the
   first value is the baseline and additional values form the sweep grid.
   Catalog defaults (shown as hints next to parameter names) are used as
   initial values when a model is selected
3. Enable/disable **Chat Quality** and **Tool Calling** scenarios
4. Set the number of **Repeats Per Test Case**
5. Click **Run Automated Testing**
6. Use **Stop** to cancel a run in progress, or **Clear Results** after
   completion

**Results:**

- A progress bar shows trial progress with elapsed time and estimated
  remaining time
- A sortable results table displays per-trial scores, TPS, TTFT, and context
  fill TPS at 20%/50%/80%
- Each row is expandable to show per-scenario, per-prompt details including
  input, expected output, actual output, usage statistics, and scoring notes
- The **Best Configuration Found** section highlights the winning trial

**Best Configuration Criteria:**

After a run completes, adjust the weights used to rank configurations (Chat
Score, Tool Score, Total Score, Avg TPS, Avg TTFT) and click **Reevaluate**
to re-rank results without re-running the tests.

**Note:** When NSeqMax > 1 in config sweeps, prompts run concurrently to
measure real parallel throughput.

#### 12.6.2 Manual Mode

Manual mode provides hands-on interaction with a loaded model through three
tabs. A session must be created before using any tab.

**Steps:**

1. Configure the model parameters
2. Click **Create Session** to load the model
3. The effective configuration is displayed after creation
4. Use the tabs below for testing
5. Click **Unload Session** to release the model when finished

**Basic Chat Tab:**

Interactive streaming chat with full control over generation parameters:

- **System Prompt** — Editable system message
- **Generation** — Temperature, Top P, Top K, Min P, Max Tokens
- **Repetition Control** — Repeat Penalty, Repeat Last N, Frequency Penalty,
  Presence Penalty
- **DRY Sampler** — DRY Multiplier, DRY Base, DRY Allowed Length, DRY Penalty
  Last N
- **XTC Sampler** — XTC Probability, XTC Threshold, XTC Min Keep
- **Reasoning** — Enable Thinking (on/off), Reasoning Effort
  (none/minimal/low/medium/high)

Messages stream in real-time with tokens-per-second displayed after each
response. A warmup request runs before each message to ensure accurate TPS
measurement.

**Tool Calling Test Tab:**

Test whether a model correctly emits tool calls:

1. Edit the **Tool Definitions** JSON (pre-populated with 10 sample tools)
2. Enter a **Test Prompt**
3. Click **Run Test**
4. Results show **PASS** with the emitted tool calls (function names and
   arguments) or **NO TOOL CALLS** with the model's text output

**Prompt Inspector Tab:**

Examine how the chat template renders messages into the prompt sent to the
model:

1. Enter a **Test Message**
2. Click **Render Prompt**
3. The fully rendered prompt text (system prompt + test message) is displayed
   with a **Copy** button

This is useful for debugging chat template formatting or verifying that
system prompts are rendered correctly for a given template.

**Export to Catalog:**

Click **Export to Catalog Editor** (in the header) to pre-fill a catalog entry
with the playground's current model, template, and configuration settings.

---

_Next: [Chapter 13: Client Integration](#chapter-13-client-integration)_
