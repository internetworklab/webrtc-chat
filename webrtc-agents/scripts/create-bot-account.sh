#!/bin/bash

set -e

BOT_USERNAME=$1
if [ -z "$BOT_USERNAME" ]; then
    echo "BOT_USERNAME (\$1) must be provided as an argument"
    exit 1
fi

BOT_DISPLAYNAME=$2
if [ -z "$BOT_DISPLAYNAME" ]; then
    echo "BOT_DISPLAYNAME (\$2) must be provided as an argument"
    exit 1
fi

AVATAR_B64_PATH=$3
if [ -z "$AVATAR_B64_PATH" ]; then
    echo "AVATAR_B64_PATH (\$3) must be provided as an argument"
    exit 1
fi

if [ -z $MANAGEMENT_API ]; then
    echo "MANAGEMENT_API is not set"
    exit 1
fi

curl \
  --silent \
  -o - \
  --unix-socket $MANAGEMENT_API \
  --form-string "username=${BOT_USERNAME}" \
  --form-string "display_name=${BOT_DISPLAYNAME}" \
  --form "avatar_url=<${AVATAR_B64_PATH}" \
  http://localhost/bots/add | jq --raw-output '.token'

# Note: Be aware, use --form 'key=<path/to/file' instead of --form 'key=@<path/to/file'

# outputs of the api
# {"token":"<token>"}
# outputs of the script:
# <the_token_itself>

# example invocation
# export MANAGEMENT_API=/var/run/webrtc-server/management.sock
# ./path/to/the/create-bot-account.sh echo_bot EchoBot ~/Downloads/testavatar.png.dataurl
