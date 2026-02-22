package tracks

import (
	"github.com/pion/rtp"
)

type RTPPacketGenerator interface {
	GetPacket(ssrc uint32, payloadType uint8, sequenceNumber uint16, timestamp uint32) (*rtp.Packet, error)
}
