#!/bin/bash

# invocation:
# scripts/launch-test-server.sh
# and .env should be in $PWD

go run ./main.go \
  --allowed-origin=http://localhost:3000 \
  --github-login-redir-url=http://localhost:3000/api/github/login/auth \
  --kioubit-login-redir-url=http://localhost:3000/api/kioubit/login/auth \
  --login-success-redir-url=http://localhost:3000/ \
  --kioubit-login-pubkey=./kioubit-login-pubkey.pem \
  --debug=true
