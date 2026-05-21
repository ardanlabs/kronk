# ==============================================================================
# Examples

example-agent:
	cd examples && go run ./agent/...

example-audio:
	cd examples && go run ./audio/main.go

example-bucky:
	cd examples && go run ./bucky/main.go

example-chat:
	cd examples && go run ./chat/main.go

example-concurrency:
	cd examples && go run ./concurrency/main.go

example-embedding:
	cd examples && go run ./embedding/main.go

example-grammar:
	cd examples && go run ./grammar/main.go

example-pool:
	cd examples && go run ./pool/main.go

example-rag:
	cd examples && go run ./rag/main.go

example-rerank:
	cd examples && go run ./rerank/main.go

example-question:
	cd examples && go run ./question/main.go

example-response:
	cd examples && go run ./response/main.go

example-vision:
	cd examples && go run ./vision/main.go

# ------------------------------------------------------------------------------

example-yzma-step1:
	cd examples && go run ./yzma/step1/main.go

example-yzma-step2:
	cd examples && go run ./yzma/step2/main.go

example-yzma-step3:
	cd examples && go run ./yzma/step3/main.go

example-yzma-step4:
	cd examples && go run ./yzma/step4/main.go

example-yzma-step5:
	cd examples && go run ./yzma/step5/main.go

example-yzma-step6:
	cd examples && go run ./yzma/step6/main.go

example-yzma-parallel-curl1:
	curl -X POST http://localhost:8090/v1/completions \
	-H "Content-Type: application/json" \
	-d '{"prompt": "Hello, how are you?", "max_tokens": 50}'

example-yzma-parallel-curl2:
	curl -X POST http://localhost:8090/v1/completions \
	-H "Content-Type: application/json" \
	-d '{"prompt": "Hello", "max_tokens": 50, "stream": true}'

example-yzma-parallel-curl3:
	curl http://localhost:8090/v1/stats

example-yzma-parallel-load:
	for i in {1..20}; do \
		curl -s -X POST http://localhost:8090/v1/completions \
		-H "Content-Type: application/json" \
		-d "{\"prompt\": \"Request $$i: Hello\", \"max_tokens\": 30}" & \
	done; wait
