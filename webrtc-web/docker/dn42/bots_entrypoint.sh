#!/bin/bash

set -e

if [ -z $MANAGEMENT_API ]; then
    echo "MANAGEMENT_API is not set"
    exit 1
fi
echo MANAGEMENT_API: $MANAGEMENT_API

if [ -z $AVATAR_B64_PATH ]; then
    echo "AVATAR_B64_PATH is not set"
    exit 1
fi
echo AVATAR_B64_PATH: $AVATAR_B64_PATH

echo Using $(pwd) as current working directory


MANAGEMENT_API=${MANAGEMENT_API} \
AVATAR_B64_PATH=${AVATAR_B64_PATH} \
scripts/prepare-bot-tokens.sh "$@"
