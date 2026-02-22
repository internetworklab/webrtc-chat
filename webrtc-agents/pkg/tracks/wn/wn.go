package wn

import (
	"fmt"
	"math/rand"

	"github.com/hraban/opus"
	"github.com/pion/rtp"
)

type OpusWhiteNoiseGenerator struct {
	opusEncoder *opus.Encoder
	numChannels int
	pcmBuf      []int16
	encodeBuf   []byte
}

func NewOpusWhiteNoiseGenerator(opusEncoder *opus.Encoder, numChannels int, samplesPerPacket int, maximumPayloadSize int) (*OpusWhiteNoiseGenerator, error) {
	whGen := &OpusWhiteNoiseGenerator{
		opusEncoder: opusEncoder,
		numChannels: numChannels,
		pcmBuf:      make([]int16, samplesPerPacket*numChannels),
		encodeBuf:   make([]byte, maximumPayloadSize),
	}

	return whGen, nil
}

func (track *OpusWhiteNoiseGenerator) GetPacket(ssrc uint32, payloadType uint8, sequenceNumber uint16, timestamp uint32) (*rtp.Packet, error) {
	pcmBuf := track.pcmBuf
	for i := range pcmBuf {
		pcmBuf[i] = int16(rand.Uint32())
	}

	encodeBuf := track.encodeBuf
	n, err := track.opusEncoder.Encode(pcmBuf, encodeBuf)
	if err != nil {
		return nil, fmt.Errorf("failed to encode payload: %w", err)
	}

	rtpHeader := rtp.Header{
		Version:        2,
		PayloadType:    payloadType,
		SequenceNumber: sequenceNumber,
		Timestamp:      timestamp,
		SSRC:           ssrc,
	}

	return &rtp.Packet{
		Header:  rtpHeader,
		Payload: encodeBuf[:n],
	}, nil
}
