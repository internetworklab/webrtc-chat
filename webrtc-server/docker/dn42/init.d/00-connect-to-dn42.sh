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

ipCMD1="nsenter -t $pid1 -n ip"
ipCMD2="nsenter -t $pid2 -n ip"

# Idempotency: check if veth interfaces already exist and remove them
if $ipCMD2 l show v-dn42 &>/dev/null || $ipCMD1 l show v-sserver &>/dev/null; then
    echo "veth pair already exists, removing for clean recreation..."
    $ipCMD2 l del v-dn42 2>/dev/null || true
    $ipCMD1 l del v-sserver 2>/dev/null || true
fi

ip l add v-dn42 netns $pid2 type veth peer v-sserver netns $pid1
if [ $? -ne 0 ]; then
    echo "Error: Failed to create veth pair"
    exit 1
fi

$ipCMD2 l set v-dn42 up
$ipCMD2 a flush scope link dev v-dn42
$ipCMD2 a add fe80::2/64 dev v-dn42
$ipCMD2 a add $DN42_ULA_ADDR/128 dev v-dn42
$ipCMD2 route replace fd00::/8 via fe80::1 dev v-dn42

$ipCMD1 l set v-sserver vrf vrf42
$ipCMD1 l set v-sserver up
$ipCMD1 a flush scope link dev v-sserver
$ipCMD1 a add fe80::1/64 dev v-sserver
$ipCMD1 route replace $DN42_ULA_ADDR/128 via fe80::2 dev v-sserver vrf vrf42

echo "Successfully executed $(basename "$0")"
