#!/bin/bash

# invocation:
# scripts/launch-test-server.sh
# and .env should be in $PWD

go run ./main.go \
  --allowed-origin=http://localhost:3000 \
  --github-login-redir-url=http://localhost:3000/api/login/auth \
  --debug=true
