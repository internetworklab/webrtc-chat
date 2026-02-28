#!/bin/bash

for script in deinit.d/*.sh; do
  $script
done

docker compose down
