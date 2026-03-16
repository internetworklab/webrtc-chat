#!/bin/bash

set -e

source .env

# Validate required environment variables

if [ -z "${DN42_ULA}" ]; then
    echo "DN42_ULA is not set or empty"
    exit 1
fi

if [ -z "$DN42_ROUTER_CONTAINER" ]; then
    echo "Error: DN42_ROUTER_CONTAINER environment variable is not set or is empty"
    exit 1
fi

if [ -z "$DN42_SUBNET" ]; then
    echo "Error: DN42_SUBNET environment variable is not set or is empty"
    exit 1
fi

br_name=$(ip --json -6 a show to "${DN42_SUBNET}" | jq --raw-output '.[].ifname')
if [ -z "${br_name}" ]; then
    echo "Can't find br name"
    exit 1
fi
echo br_name: "${br_name}"

pid1=$(docker inspect $DN42_ROUTER_CONTAINER -f {{.State.Pid}} 2>/dev/null)
if [ -z "$pid1" ] || [ "$pid1" -eq 0 ] 2>/dev/null; then
    echo "Error: Failed to get PID for container '$DN42_ROUTER_CONTAINER'. Container may not exist or is not running."
    exit 1
fi
echo pid1: "${pid1}"

ipCMD1="nsenter -t $pid1 -n ip"
inject_if_name="v-chat"
table_id="1101"
ip_rule_pref="32765"

# Idempotency: check if veth interfaces already exist and remove them
if ip l show "${inject_if_name}" &>/dev/null; then
    echo "Injector veth pair already exists, removing for clean re-creation..."
    ip l del "${inject_if_name}"
fi

echo "Creating injector veth pair ${inject_if_name} (host) to ${inject_if_name} (in router)"
ip l add "${inject_if_name}" type veth peer "${inject_if_name}" netns "${pid1}"

ip l set "${inject_if_name}" master "${br_name}"
ip l set "${inject_if_name}" up
ip -6 rule add from "${DN42_SUBNET}" table "${table_id}" pref "${ip_rule_pref}" &>/dev/null || true
ip route add fd00::/8 via fe80::1 dev "${br_name}" table "${table_id}" &>/dev/null || true

$ipCMD1 l set "${inject_if_name}" vrf vrf42
$ipCMD1 l set "${inject_if_name}" up
$ipCMD1 a add fe80::1/64 dev "${inject_if_name}"
$ipCMD1 route add "${DN42_ULA}/128" dev "${inject_if_name}" vrf vrf42
