##@ SDK examples

# ==============================================================================
# Examples

example-agent: ## Run the agent example
	cd examples && go run ./agent/...

example-audio: ## Run the audio example
	cd examples && go run ./audio/main.go

example-bucky: ## Run the bucky example
	cd examples && go run ./bucky/main.go

example-bucky-stream: ## Run the bucky stream example
	cd examples && go run ./bucky-stream/main.go $(ARGS)

example-bucky-stream-vad: ## Run the bucky stream vad example
	$(MAKE) example-bucky-stream ARGS="--silero-vad --vad-threshold=0.5"

example-bucky-diar: ## Run the bucky diar example
	cd examples && go run ./bucky-diar/main.go

example-chat: ## Run the chat example
	cd examples && go run ./chat/main.go

example-concurrency: ## Run the concurrency example
	cd examples && go run ./concurrency/main.go

example-decision: ## Run the decision example
	cd examples && go run ./decision/main.go

example-decision-audio: ## Run the decision audio example
	cd examples && go run ./decision-audio/main.go

example-decision-image: ## Run the decision image example
	cd examples && go run ./decision-image/main.go

example-embedding: ## Run the embedding example
	cd examples && go run ./embedding/main.go

example-grammar: ## Run the grammar example
	cd examples && go run ./grammar/main.go

example-malina: ## Run the malina example
	cd examples && go run ./malina/main.go

example-malina-img2img: ## Run the malina img2img example
	cd examples && go run ./malina-img2img/main.go

example-malina-sd-encode: ## Run the malina sd encode example
	cd examples && go run ./malina-sd-encode/main.go

example-malina-system: ## Run the malina system example
	cd examples && go run ./malina-system/main.go

example-malina-controlnet: ## Run the malina controlnet example
	cd examples && go run ./malina-controlnet/main.go

example-malina-upscale: ## Run the malina upscale example
	cd examples && go run ./malina-upscale/main.go

example-malina-adetailer: ## Run the malina adetailer example
	cd examples && go run ./malina-adetailer/main.go

example-malina-animatediff: ## Run the malina animatediff example
	cd examples && go run ./malina-animatediff/main.go

example-malina-s2v: ## Run the malina s2v example
	cd examples && go run ./malina-s2v/main.go

example-pool: ## Run the pool example
	cd examples && go run ./pool/main.go

example-rag: ## Run the rag example
	cd examples && go run ./rag/main.go

example-rerank: ## Run the rerank example
	cd examples && go run ./rerank/main.go

example-question: ## Run the question example
	cd examples && go run ./question/main.go

example-response: ## Run the response example
	cd examples && go run ./response/main.go

example-session-store: ## Run the session store example
	cd examples && go run ./session-store/...

example-vision: ## Run the vision example
	cd examples && go run ./vision/main.go

# ------------------------------------------------------------------------------

example-yzma-step1: ## Run the yzma step1 example
	cd examples && go run ./yzma/step1/main.go

example-yzma-step2: ## Run the yzma step2 example
	cd examples && go run ./yzma/step2/main.go

example-yzma-step3: ## Run the yzma step3 example
	cd examples && go run ./yzma/step3/main.go

example-yzma-step4: ## Run the yzma step4 example
	cd examples && go run ./yzma/step4/main.go

example-yzma-step5: ## Run the yzma step5 example
	cd examples && go run ./yzma/step5/main.go

example-yzma-step6: ## Run the yzma step6 example
	cd examples && go run ./yzma/step6/main.go

example-yzma-step7: ## Run the yzma step7 example
	cd examples && go run ./yzma/step7/main.go

example-yzma-step8: ## Run the yzma step8 example
	cd examples && go run ./yzma/step8/main.go

example-yzma-step9: ## Run the yzma step9 example
	cd examples && go run ./yzma/step9/main.go

example-yzma-parallel-curl1: ## Run the yzma parallel curl1 example
	curl -X POST http://localhost:8090/v1/completions \
	-H "Content-Type: application/json" \
	-d '{"prompt": "Hello, how are you?", "max_tokens": 50}'

example-yzma-parallel-curl2: ## Run the yzma parallel curl2 example
	curl -X POST http://localhost:8090/v1/completions \
	-H "Content-Type: application/json" \
	-d '{"prompt": "Hello", "max_tokens": 50, "stream": true}'

example-yzma-parallel-curl3: ## Run the yzma parallel curl3 example
	curl http://localhost:8090/v1/stats

example-yzma-parallel-load: ## Run the yzma parallel load example
	for i in {1..20}; do \
		curl -s -X POST http://localhost:8090/v1/completions \
		-H "Content-Type: application/json" \
		-d "{\"prompt\": \"Request $$i: Hello\", \"max_tokens\": 30}" & \
	done; wait
