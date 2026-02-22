package wn

import (
	"fmt"
	"math"

	"github.com/hraban/opus"
	"github.com/pion/rtp"
)

type OpusSineWaveformGenerator struct {
	opusEncoder *opus.Encoder
	numChannels int
	pcmBuf      []int16
	encodeBuf   []byte
	frequency   float64
	sampleRate  int
}

func NewOpusSineWaveformGenerator(
	opusEncoder *opus.Encoder,
	numChannels int,
	samplesPerPacket int,
	maximumPayloadSize int,
	frequency float64,
	sampleRate int,
) (*OpusSineWaveformGenerator, error) {
	whGen := &OpusSineWaveformGenerator{
		opusEncoder: opusEncoder,
		numChannels: numChannels,
		pcmBuf:      make([]int16, samplesPerPacket*numChannels),
		encodeBuf:   make([]byte, maximumPayloadSize),
		frequency:   frequency,
		sampleRate:  sampleRate,
	}

	if numChannels < 1 {
		return nil, fmt.Errorf("invalid number of channels")
	}

	return whGen, nil
}

func (generator *OpusSineWaveformGenerator) GetPacket(ssrc uint32, payloadType uint8, sequenceNumber uint16, timestamp uint32) (*rtp.Packet, error) {
	pcmBuf := generator.pcmBuf
	for i := 0; i < len(pcmBuf)/generator.numChannels; i++ {
		t := float64(timestamp+uint32(i)) / float64(generator.sampleRate)
		x := 2 * math.Pi * generator.frequency * t
		y := math.Sin(x)
		scaledY := math.Round(y * float64(math.MaxInt16))
		roundedY := int16(scaledY)
		for j := range generator.numChannels {
			pcmBuf[i*generator.numChannels+j] = roundedY
		}
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
