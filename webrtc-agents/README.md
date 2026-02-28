## Notes

### About the dependencies

This project relies on [hraban/opus](https://github.com/hraban/opus) golang library and libopus to compile, install the dependencies before trying to build the project:

On Debian, Ubuntu, ...:

```sh
sudo apt-get install pkg-config libopus-dev libopusfile-dev
```

On macOS:

```sh
brew install pkg-config opus opusfile
```

### Build

To build this multi-arch image, use the following commands from the `webrtc-demo` root directory:

```bash
# Build and push multi-arch image  
docker buildx build \
 --platform linux/arm64 --platform linux/amd64 \
 --file webrtc-agents/Dockerfile \
 --tag yourrepo.com/username/webrtc-agents:latest \
 --push \
 .

# Or build for local testing (single arch)
docker buildx build \
 --platform linux/arm64 --platform linux/amd64 \
 --file webrtc-agents/Dockerfile \
 --tag webrtc-agents:latest \
 --load \
 .
```
