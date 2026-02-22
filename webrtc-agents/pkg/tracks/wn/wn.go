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
	name        string
}

func NewOpusWhiteNoiseGenerator(name string, opusEncoder *opus.Encoder, numChannels int, samplesPerPacket int, maximumPayloadSize int) (*OpusWhiteNoiseGenerator, error) {
	whGen := &OpusWhiteNoiseGenerator{
		name:        name,
		opusEncoder: opusEncoder,
		numChannels: numChannels,
		pcmBuf:      make([]int16, samplesPerPacket*numChannels),
		encodeBuf:   make([]byte, maximumPayloadSize),
	}

	return whGen, nil
}

func (generator *OpusWhiteNoiseGenerator) GetPacket(ssrc uint32, payloadType uint8, sequenceNumber uint16, timestamp uint32) (*rtp.Packet, error) {
	pcmBuf := generator.pcmBuf
	for i := range pcmBuf {
		pcmBuf[i] = int16(rand.Uint32())
	}

	encodeBuf := generator.encodeBuf
	n, err := generator.opusEncoder.Encode(pcmBuf, encodeBuf)
	if err != nil {
		return nil, fmt.Errorf("failed to encode payload: %w\nssrc=%d,payloadType=%d,len(pcmBuf)=%d,len(encodeBuf)=%d", err, ssrc, payloadType, len(pcmBuf), len(encodeBuf))
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

func (generator *OpusWhiteNoiseGenerator) GetName() string {
	return generator.name
}
