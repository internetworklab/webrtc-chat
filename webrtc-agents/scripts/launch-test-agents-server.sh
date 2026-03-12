#!/bin/bash

set -e

current_path=$(realpath $0)
cd $(dirname $current_path)/..

echo Using $(pwd) as current working directory

if [ -z $MANAGEMENT_API ]; then
    echo "MANAGEMENT_API is not set"
    exit 1
fi

if [ -z $AVATAR_B64_PATH ]; then
    echo "AVATAR_B64_PATH is not set"
    exit 1
fi

echo "Getting Bot JWT token for EchoBot ..."
echobot_token=$(MANAGEMENT_API=$MANAGEMENT_API scripts/create-bot-account.sh echo_bot EchoBot $AVATAR_B64_PATH)
echo "Done, got ${echobot_token}"

echo "Getting Bot JWT token for MusicBot ..."
musicbot_token=$(MANAGEMENT_API=$MANAGEMENT_API scripts/create-bot-account.sh music_bot MusicBot $AVATAR_B64_PATH)
echo "Done, got ${musicbot_token}"

echo "Getting Bot JWT token for CounterBot ..."
counterbot_token=$(MANAGEMENT_API=$MANAGEMENT_API scripts/create-bot-account.sh counter_bot CounterBot $AVATAR_B64_PATH)
echo "Done, got ${counterbot_token}"

echo "Getting Bot JWT token for ClockBot ..."
clockbot_token=$(MANAGEMENT_API=$MANAGEMENT_API scripts/create-bot-account.sh clock_bot ClockBot $AVATAR_B64_PATH)
echo "Done, got ${clockbot_token}"

echo "Getting Bot JWT token for ChatBot ..."
chatbot_token=$(MANAGEMENT_API=$MANAGEMENT_API scripts/create-bot-account.sh chat_bot ChatBot $AVATAR_B64_PATH)
echo "Done, got ${chatbot_token}"

echo "Generating .env.bot file in current directory $PWD"
echo "ECHOBOT_JWT_TOKEN=$echobot_token" > .env.bots
echo "MUSICBOT_JWT_TOKEN=$musicbot_token" >> .env.bots
echo "COUNTERBOT_JWT_TOKEN=$counterbot_token" >> .env.bots
echo "CLOCKBOT_JWT_TOKEN=$clockbot_token" >> .env.bots
echo "CHATBOT_JWT_TOKEN=$chatbot_token" >> .env.bots

echo "Done, launching bot agents process ..."

# invocation:
# scripts/launch-test-agents-server.sh
# and .env should be in $PWD

go run ./cmd/agents/main.go \
  --chatbot-model=deepseek/deepseek-v3.2
