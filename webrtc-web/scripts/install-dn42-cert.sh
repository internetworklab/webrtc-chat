#!/bin/bash

domain="chat.duststars.dn42"

# install cert for lounge
docker run \
  --rm \
  -it \
  -v chat_acme_state:/acme.sh \
  -v chat_acme_certs:/etc/nginx/acme-certs \
  docker.io/neilpang/acme.sh:latest \
    acme.sh \
    --install-cert \
    --domain $domain \
    --fullchain-file /etc/nginx/acme-certs/chat.duststars.dn42.crt \
    --key-file /etc/nginx/acme-certs/chat.duststars.dn42.key
