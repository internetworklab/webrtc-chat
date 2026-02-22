package tracks

import (
	"github.com/pion/rtp"
)

type RTPPacketGenerator interface {
	// ssrc, payloadType, sequenceNumber are those appearing in the RTP header.
	// timestamp is in terms of the sample index,
	// for example, when sampleRate is 48000, timestamp=12000 would means 12000/48000 = 0.25 seconds
	GetPacket(ssrc uint32, payloadType uint8, sequenceNumber uint16, timestamp uint32) (*rtp.Packet, error)

	GetName() string
}
