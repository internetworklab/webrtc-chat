package tracks

import (
	"github.com/pion/rtp"
)

// RTPPacketGenerator is an interface for generating RTP packets.
//
// Codec Requirements:
// This package is designed to work exclusively with Opus codec at 48kHz sample rate
// with 2 channels (stereo). The MediaEngine used in WebRTC peer connections must be
// configured to register only Opus codec with these parameters:
//
//	MimeType:    webrtc.MimeTypeOpus
//	ClockRate:   48000
//	Channels:    2
//	SDPFmtpLine: "minptime=10;useinbandfec=1"
//
// If the negotiated codec differs from Opus 48kHz stereo, track binding will fail.
// See SignallingHandler in pkg/handlers for proper MediaEngine configuration.
type RTPPacketGenerator interface {
	// ssrc, payloadType, sequenceNumber are those appearing in the RTP header.
	// timestamp is in terms of the sample index,
	// for example, when sampleRate is 48000, timestamp=12000 would means 12000/48000 = 0.25 seconds
	GetPacket(ssrc uint32, payloadType uint8, sequenceNumber uint16, timestamp uint32) (*rtp.Packet, error)

	GetName() string
}
