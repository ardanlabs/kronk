# ==============================================================================
# Running OpenWebUI

owu-up:
	docker compose -f zarf/docker/compose.yaml up openwebui

owu-down:
	docker compose -f zarf/docker/compose.yaml down openwebui

owu-browse:
	$(OPEN_CMD) http://localhost:8081/

# ==============================================================================
# Metrics and Tracing

UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Darwin)
	OPEN_CMD := open
else
	OPEN_CMD := xdg-open
endif

website:
	$(OPEN_CMD) http://localhost:11435/

statsviz:
	$(OPEN_CMD) http://localhost:11445/debug/statsviz

grafana-up:
	docker compose -f zarf/docker/compose.yaml up grafana loki prometheus promtail tempo

grafana-down:
	docker compose -f zarf/docker/compose.yaml down grafana loki prometheus promtail tempo

grafana-browse:
	$(OPEN_CMD) http://localhost:3100/

# ==============================================================================
# Debugging

debug-responses-qwen:
	curl -s http://localhost:11435/v1/responses -H "Content-Type: application/json" -d '{"model":"Qwen3.5-35B-A3B-UD-Q8_K_XL","stream":false,"instructions":"You are a helpful assistant.","input":"Create a file called test.txt","tools":[{"type":"function","name":"editor","description":"Create or edit files","parameters":{"type":"object","properties":{"path":{"type":"string"},"new_text":{"type":"string"}},"required":["path","new_text"]}}]}' | python3 -m json.tool

debug-responses-gemma:
	curl -s http://localhost:11435/v1/responses -H "Content-Type: application/json" -d '{"model":"gemma-4-26B-A4B-it-UD-Q8_K_XL","stream":false,"instructions":"You are a helpful assistant.","input":"Create a file called test.txt","tools":[{"type":"function","name":"editor","description":"Create or edit files","parameters":{"type":"object","properties":{"path":{"type":"string"},"new_text":{"type":"string"}},"required":["path","new_text"]}}]}' | python3 -m json.tool

debug-completions-qwen:
	curl -s http://localhost:11435/v1/chat/completions -H "Content-Type: application/json" -d '{"model":"gemma-4-26B-A4B-it-UD-Q8_K_XL","stream":false,"messages":[{"role":"system","content":"You are a helpful assistant."},{"role":"user","content":"Please edit this file `sdk/kronk/model/yzma.go` using the `tool_go_code_editor` tool and add a comment to the top that says \"BILL WAS HERE\"."}],"tools":[{"type":"function","function":{"name":"tool_go_code_editor","description":"Edit Golang source code files including adding, replacing, and deleting lines.","parameters":{"type":"object","properties":{"path":{"type":"string","description":"Relative path and name of the Golang file"},"line_number":{"type":"integer","description":"The line number for the code change"},"type_change":{"type":"string","description":"The type of change to make: add, replace, delete"},"line_change":{"type":"string","description":"The text to add, replace, delete"}},"required":["path","line_number","type_change","line_change"]}}}]}' | python3 -m json.tool

debug-completions-gemma:
	curl -s http://localhost:11434/v1/chat/completions -H "Content-Type: application/json" -d '{"model":"gemma4:26b","stream":false,"messages":[{"role":"system","content":"You are a helpful assistant."},{"role":"user","content":"Please edit this file `sdk/kronk/model/yzma.go` using the `tool_go_code_editor` tool and add a comment to the top that says \"BILL WAS HERE\"."}],"tools":[{"type":"function","function":{"name":"tool_go_code_editor","description":"Edit Golang source code files including adding, replacing, and deleting lines.","parameters":{"type":"object","properties":{"path":{"type":"string","description":"Relative path and name of the Golang file"},"line_number":{"type":"integer","description":"The line number for the code change"},"type_change":{"type":"string","description":"The type of change to make: add, replace, delete"},"line_change":{"type":"string","description":"The text to add, replace, delete"}},"required":["path","line_number","type_change","line_change"]}}}]}' | python3 -m json.tool
