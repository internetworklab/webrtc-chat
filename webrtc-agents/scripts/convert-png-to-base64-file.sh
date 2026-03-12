#!/bin/bash

set -e

if [ -z "$1" ]; then
    exit 1
fi

dir_name=$(dirname $1)
base_name=$(basename $1)
output_basename="${base_name}.dataurl"

cd "$dir_name"

echo -n "data:image/png;base64," > $output_basename
base64 -i $base_name >> $output_basename

echo Base64-encoded file has been written to $dir_name/$output_basename
