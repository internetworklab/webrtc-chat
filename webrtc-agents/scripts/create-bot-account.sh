#!/bin/bash

MANAGEMENT_API=$(pwd)/management/management.sock

curl \
  --unix-socket $MANAGEMENT_API \
  --form-string 'username=echo_bot' \
  --form-string 'display_name=EchoBot' \
  --form 'avatar_url=<testavatar.png.dataurl' \
  http://localhost/bots/add

# Note: Be aware, use --form 'key=<path/to/file' instead of --form 'key=@<path/to/file'

# outputs
# {"token":"<token>"}
