# Chapter 13: Client Integration

## Table of Contents

- [13.1 OpenWebUI](#131-openwebui)
- [13.2 Cline](#132-cline)
- [13.4 Python OpenAI SDK](#134-python-openai-sdk)
- [13.5 curl and HTTP Clients](#135-curl-and-http-clients)
- [13.6 LangChain](#136-langchain)

---

Kronk's OpenAI-compatible API works with popular AI clients and tools.

### 13.1 OpenWebUI

OpenWebUI is a self-hosted chat interface that works with Kronk.

**Configure OpenWebUI:**

1. Open OpenWebUI settings
2. Navigate to Connections → OpenAI API
3. Set the base URL:

```
http://localhost:8080/v1
```

4. Set API key to your Kronk token (or any value (123) if auth is disabled)
5. Save and refresh models

**Features that work:**

- Chat completions with streaming
- Model selection from available models
- System prompts
- Conversation history

### 13.2 Cline

Cline is a VS Code extension for AI-assisted coding.

**Configure Cline for Kronk:**

1. Open VS Code settings
2. Search for "Cline"
3. Set API Provider to "OpenAI Compatible"
4. Configure:

```
Base URL: http://localhost:8080/v1
API Key: <your-kronk-token> or 123 for anything
Model: Qwen_Qwen3.5-35B-A3B-Q8_0/IMC
```

**Recommended Model Settings:**

For coding tasks, configure your model with:

```yaml
models:
    Qwen_Qwen3.5-35B-A3B-Q8_0:
    &base_Qwen_Qwen3.5-35B-A3B-Q8_0
    context-window: 131072
    nbatch: 2048
    nubatch: 512
    cache-type-k: q8_0
    cache-type-v: q8_0
    flash-attention: enabled
    nseq-max: 2
    insecure-logging: true
    sampling-parameters:
        temperature: 0.7
        top_p: 0.8
        top_k: 20

    Qwen_Qwen3.5-35B-A3B-Q8_0/IMC:
    <<: *base_Qwen_Qwen3.5-35B-A3B-Q8_0
    nseq-max: 1
    incremental-cache: true
```

IMC is especially beneficial for Cline's iterative coding workflow.

_Note: Don't use R1 Message formats when using KMS._

### 13.4 Python OpenAI SDK

Use the official OpenAI Python library with Kronk.

**Installation:**

```shell
pip install openai
```

**Usage:**

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:8080/v1",
    api_key="your-kronk-token"  # Or any string if auth disabled
)

response = client.chat.completions.create(
    model="Qwen_Qwen3.5-35B-A3B-Q8_0/IMC",
    messages=[
        {"role": "system", "content": "You are a helpful assistant."},
        {"role": "user", "content": "Hello!"}
    ],
    stream=True
)

for chunk in response:
    if chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="")
```

### 13.5 curl and HTTP Clients

Any HTTP client can call Kronk's REST API directly.

**Basic Request:**

```shell
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -d '{
    "model": "Qwen_Qwen3.5-35B-A3B-Q8_0",
    "messages": [{"role": "user", "content": "Hello"}],
    "stream": true
  }'
```

**Streaming Response:**

Streaming responses use Server-Sent Events (SSE) format:

```
data: {"id":"...","choices":[{"delta":{"content":"Hello"}}],...}

data: {"id":"...","choices":[{"delta":{"content":"!"}}],...}

data: [DONE]
```

### 13.6 LangChain

Use LangChain with Kronk via the OpenAI integration.

**Installation:**

```shell
pip install langchain-openai
```

**Usage:**

```python
from langchain_openai import ChatOpenAI

llm = ChatOpenAI(
    base_url="http://localhost:8080/v1",
    api_key="your-kronk-token",
    model="Qwen_Qwen3.5-35B-A3B-Q8_0",
    streaming=True
)

response = llm.invoke("Explain quantum computing briefly.")
print(response.content)
```

---

_Next: [Chapter 14: Observability](#chapter-14-observability)_
