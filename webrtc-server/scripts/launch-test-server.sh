#!/bin/bash

# invocation:
# scripts/launch-test-server.sh
# and .env should be in $PWD

go run ./main.go \
  --inject-allow-all-cors-headers \
  --allowed-origin=http://localhost:3000
