##@ Operations and observability

# ==============================================================================
# Running OpenWebUI

owu-up: ## Start Open WebUI
	docker compose -f zarf/docker/compose.yaml up openwebui

owu-down: ## Stop Open WebUI
	docker compose -f zarf/docker/compose.yaml down openwebui

owu-browse: ## Open Open WebUI in a browser
	$(OPEN_CMD) http://localhost:8081/

# ==============================================================================
# Running Kronk in a container
#
# GPU: make kronk-up KRONK_GPU=cuda   (cuda | rocm | vulkan)
# Server settings in zarf/docker/kronk/compose.override.yaml are added when
# the file exists.

KRONK_COMPOSE := -f zarf/docker/kronk/compose$(if $(KRONK_GPU),.$(KRONK_GPU)).yaml \
	$(if $(wildcard zarf/docker/kronk/compose.override.yaml),-f zarf/docker/kronk/compose.override.yaml)

kronk-up: ## Start the Kronk Docker Compose stack
	docker compose $(KRONK_COMPOSE) up -d

kronk-down: ## Stop the Kronk Docker Compose stack
	docker compose $(KRONK_COMPOSE) down

kronk-logs: ## Follow Kronk container logs
	docker compose $(KRONK_COMPOSE) logs -f kronk

# ==============================================================================
# Metrics and Tracing

UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Darwin)
	OPEN_CMD := open
else
	OPEN_CMD := xdg-open
endif

website: ## Open the Kronk browser UI
	$(OPEN_CMD) http://localhost:11435/

statsviz: ## Open runtime statistics
	$(OPEN_CMD) http://localhost:11445/debug/statsviz

grafana-up: ## Start the observability stack
	docker compose -f zarf/docker/compose.yaml up grafana loki prometheus alloy tempo

grafana-down: ## Stop the observability stack
	docker compose -f zarf/docker/compose.yaml down grafana loki prometheus alloy tempo

grafana-browse: ## Open Grafana
	$(OPEN_CMD) http://localhost:3100/

# ==============================================================================
# Debugging

debug-responses-qwen: ## Debug responses qwen requests
	curl -s http://localhost:11435/v1/responses -H "Content-Type: application/json" -d '{"model":"Qwen3.5-35B-A3B-UD-Q8_K_XL","stream":false,"instructions":"You are a helpful assistant.","input":"Create a file called test.txt","tools":[{"type":"function","name":"editor","description":"Create or edit files","parameters":{"type":"object","properties":{"path":{"type":"string"},"new_text":{"type":"string"}},"required":["path","new_text"]}}]}' | python3 -m json.tool

debug-responses-gemma: ## Debug responses gemma requests
	curl -s http://localhost:11435/v1/responses -H "Content-Type: application/json" -d '{"model":"gemma-4-26B-A4B-it-UD-Q8_K_XL","stream":false,"instructions":"You are a helpful assistant.","input":"Create a file called test.txt","tools":[{"type":"function","name":"editor","description":"Create or edit files","parameters":{"type":"object","properties":{"path":{"type":"string"},"new_text":{"type":"string"}},"required":["path","new_text"]}}]}' | python3 -m json.tool

debug-completions-qwen: ## Debug completions qwen requests
	curl -s http://localhost:11435/v1/chat/completions -H "Content-Type: application/json" -d '{"model":"gemma-4-26B-A4B-it-UD-Q8_K_XL","stream":false,"messages":[{"role":"system","content":"You are a helpful assistant."},{"role":"user","content":"Please edit this file `sdk/kronk/model/yzma.go` using the `tool_go_code_editor` tool and add a comment to the top that says \"BILL WAS HERE\"."}],"tools":[{"type":"function","function":{"name":"tool_go_code_editor","description":"Edit Golang source code files including adding, replacing, and deleting lines.","parameters":{"type":"object","properties":{"path":{"type":"string","description":"Relative path and name of the Golang file"},"line_number":{"type":"integer","description":"The line number for the code change"},"type_change":{"type":"string","description":"The type of change to make: add, replace, delete"},"line_change":{"type":"string","description":"The text to add, replace, delete"}},"required":["path","line_number","type_change","line_change"]}}}]}' | python3 -m json.tool

debug-completions-gemma: ## Debug completions gemma requests
	curl -s http://localhost:11434/v1/chat/completions -H "Content-Type: application/json" -d '{"model":"gemma4:26b","stream":false,"messages":[{"role":"system","content":"You are a helpful assistant."},{"role":"user","content":"Please edit this file `sdk/kronk/model/yzma.go` using the `tool_go_code_editor` tool and add a comment to the top that says \"BILL WAS HERE\"."}],"tools":[{"type":"function","function":{"name":"tool_go_code_editor","description":"Edit Golang source code files including adding, replacing, and deleting lines.","parameters":{"type":"object","properties":{"path":{"type":"string","description":"Relative path and name of the Golang file"},"line_number":{"type":"integer","description":"The line number for the code change"},"type_change":{"type":"string","description":"The type of change to make: add, replace, delete"},"line_change":{"type":"string","description":"The text to add, replace, delete"}},"required":["path","line_number","type_change","line_change"]}}}]}' | python3 -m json.tool
