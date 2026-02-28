#!/bin/bash

docker compose up -d

for script in init.d/*.sh; do
    $script
done
