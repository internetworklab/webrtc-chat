module webrtc-agents

go 1.24.6

require example.com/webrtcserver v0.0.0

require github.com/alecthomas/kong v1.14.0 // indirect

require (
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/gorilla/websocket v1.5.3
	github.com/quic-go/quic-go v0.59.0 // indirect
	golang.org/x/crypto v0.41.0 // indirect
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
)

replace example.com/webrtcserver => ../webrtc-server
