#!/bin/bash

source .env

# Validate required environment variables
if [ -z "$DN42_ROUTER_CONTAINER" ]; then
    echo "Error: DN42_ROUTER_CONTAINER environment variable is not set or is empty"
    exit 1
fi

if [ -z "$DN42_ULA_ADDR" ]; then
    echo "Error: DN42_ULA_ADDR environment variable is not set or is empty"
    exit 1
fi

if [ -z "$CONTAINER_NAME" ]; then
    echo "Error: CONTAINER_NAME environment variable is not set or is empty"
    exit 1
fi


pid1=$(docker inspect $DN42_ROUTER_CONTAINER -f {{.State.Pid}} 2>/dev/null)
if [ -z "$pid1" ] || [ "$pid1" -eq 0 ] 2>/dev/null; then
    echo "Error: Failed to get PID for container '$DN42_ROUTER_CONTAINER'. Container may not exist or is not running."
    exit 1
fi

pid2=$(docker inspect $CONTAINER_NAME -f {{.State.Pid}} 2>/dev/null)
if [ -z "$pid2" ] || [ "$pid2" -eq 0 ] 2>/dev/null; then
    echo "Error: Failed to get PID for container '$CONTAINER_NAME'. Container may not exist or is not running."
    exit 1
fi
