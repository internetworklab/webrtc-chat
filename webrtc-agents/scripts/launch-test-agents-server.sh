#!/bin/bash

# invocation:
# scripts/launch-test-agents-server.sh
# and .env should be in $PWD

go run ./cmd/agents/main.go \
  --chatbot-model=deepseek/deepseek-v3.2
