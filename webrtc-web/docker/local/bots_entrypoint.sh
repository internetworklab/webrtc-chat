#!/bin/sh


# Note: the purpose of this script is to register
# bots' user accounts to the server, obtain the JWT tokens
# and programatically generate the .env.bot environment file for
# keeping the JWT token secrets so that the bots can authenticate
# thenselves to the server using Authorization bearer method.
#
# This script relys on: curl, jq, and assuming the management api
# socket of the server is in /var/run/webrtc-server/management.sock
#
# also this script is assuming to run on the working directory as same
# as the agents process's working directory.

set -e

MANAGEMENT_API=/var/run/webrtc-server/management.sock

echo "Getting Bot JWT token for EchoBot ..."
echobot_token=$(echo todo)

echo "Getting Bot JWT token for MusicBot ..."
musicbot_token=$(echo todo)

echo "Getting Bot JWT token for CounterBot ..."
counterbot_token=$(echo todo)

echo "Getting Bot JWT token for ClockBot ..."
clockbot_token=$(echo todo)

echo "Getting Bot JWT token for ChatBot ..."
chatbot_token=$(echo todo)

echo "Generating .env.bot file in current directory $PWD"
echo "ECHOBOT_JWT_TOKEN=$echobot_token" > .env.bots
echo "MUSICBOT_JWT_TOKEN=$musicbot_token" >> .env.bots
echo "COUNTERBOT_JWT_TOKEN=$counterbot_token" >> .env.bots
echo "CLOCKBOT_JWT_TOKEN=$clockbot_token" >> .env.bots
echo "CHATBOT_JWT_TOKEN=$chatbot_token" >> .env.bots

echo "Done, launching bot agents process ..."
exec /app/agents $@
