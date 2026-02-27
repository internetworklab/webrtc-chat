## Build

In this directory:

```bash
docker buildx build \
 --platform linux/arm64 --platform linux/amd64 \
 --tag webrtc-server:latest \
 --load \
 .
```
