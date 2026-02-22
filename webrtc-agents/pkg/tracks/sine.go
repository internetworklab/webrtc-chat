package tracks

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"github.com/pion/rtp"
	webrtc "github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media/oggwriter"
)

// TrackHandle implements a TrackLocal interface
type TrackHandle struct {
	trackId               string
	streamId              string
	stopChan              chan struct{}
	sampleRate            int
	numChannels           int
	samplesPerPacket      int
	numPrePopulatePackets int
	packetGen             RTPPacketGenerator
	frameIntv             time.Duration
}

const DefaultMTU = 1280 // 1280 is the minimum requirement for IPv6 over ethernet
const DefaultMaxPayloadSize = DefaultMTU -
	40 - // IPv6 header
	8 - // UDP header
	16 - // SRTP Auth Tag, would be 10 bytes (for HMAC-SHA1), or 16 bytes (for AES-GCM)
	12 - // minimum RTP header of fixed size
	20 // possible RTP extensions

// The Opus encoder can output encoded frames representing 2.5, 5, 10,
// 20, 40, or 60 ms of speech or audio data.
// we would use 20ms here, which would be (20ms/1000ms)*(48000samples/s)=960 samples per packet
const DefaultFrameIntv = 20 * time.Millisecond
const DefaultSampleRate = 48000
const DefaultChannelsCount = 2

func NewTrackHandle(streamId string, frameIntv time.Duration, sampleRate int, numChannels int, packetGen RTPPacketGenerator) (*TrackHandle, error) {

	wh := &TrackHandle{
		trackId:               fmt.Sprintf("audio-%s", uuid.New().String()),
		streamId:              streamId,
		stopChan:              make(chan struct{}),
		sampleRate:            sampleRate,
		numChannels:           numChannels,
		numPrePopulatePackets: 10,
		frameIntv:             frameIntv,
		packetGen:             packetGen,
	}

	wh.samplesPerPacket = int(float64(wh.frameIntv.Seconds()) * float64(wh.sampleRate))

	return wh, nil
}

func getOpusCodecParams(ctx webrtc.TrackLocalContext) *webrtc.RTPCodecParameters {
	for _, codec := range ctx.CodecParameters() {
		if codec.RTPCodecCapability.MimeType == webrtc.MimeTypeOpus {
			return &codec
		}
	}
	return nil
}

func (track *TrackHandle) encodeAndSend(
	ssrc uint32,
	payloadType uint8,
	writer webrtc.TrackLocalWriter,
	sequenceNumber uint16,
	timestamp uint32) (uint16, uint32, error) {
	pkt, err := track.packetGen.GetPacket(ssrc, payloadType, sequenceNumber, timestamp)
	if err != nil {
		return sequenceNumber, timestamp, fmt.Errorf("failed to get packet: %w", err)
	}

	if _, err := writer.WriteRTP(&pkt.Header, pkt.Payload); err != nil {
		return sequenceNumber, timestamp, fmt.Errorf("failed to write RTP packet: %w", err)
	}

	sequenceNumber++
	timestamp += uint32(track.samplesPerPacket)
	return sequenceNumber, timestamp, nil
}

func (track *TrackHandle) startStreaming(ctx webrtc.TrackLocalContext, selectedCodec webrtc.RTPCodecParameters) {
	// refer to [rfc7587](https://datatracker.ietf.org/doc/html/rfc7587)
	// for the browser support of codecs, refer to https://developer.mozilla.org/en-US/docs/Web/Media/Guides/Formats/WebRTC_codecs

	// sequenceNumber increases by every single packet sent, it's also the sequence number in RTP packet header
	// by RFC 3550, they should be started at random
	var sequenceNumber uint16 = uint16(rand.Uint32())

	// timestamp is the timestamp in RTP packet header,
	// "reflects the sampling instant of the first octet in the RTP data packet."
	// As an example, for fixed-rate audio
	// the timestamp clock would likely increment by one for each
	// sampling period.  If an audio application reads blocks covering
	// 160 sampling periods from the input device, the timestamp would be
	// increased by 160 for each such block, regardless of whether the
	//  block is transmitted in a packet or dropped as silent.
	// // by RFC 3550, they should be started at random
	var timestamp uint32 = rand.Uint32()

	writer := ctx.WriteStream()
	pt := uint8(selectedCodec.PayloadType)
	ssrc := uint32(ctx.SSRC())

	// pre-populating packets to the receiver's buffer
	var err error
	for i := 0; i < track.numPrePopulatePackets; i++ {
		sequenceNumber, timestamp, err = track.encodeAndSend(ssrc, pt, writer, sequenceNumber, timestamp)
		if err != nil {
			log.Println("Failed to encode and send RTP packet")
			return
		}
	}

	ticker := time.NewTicker(track.frameIntv)

	for {
		select {
		case <-ticker.C:

			sequenceNumber, timestamp, err = track.encodeAndSend(ssrc, pt, writer, sequenceNumber, timestamp)
			if err != nil {
				log.Println("Failed to encode and send RTP packet")
				return
			}

		case <-track.stopChan:
			ticker.Stop()
			return
		}
	}
}

type WrappedOGGWriter struct {
	OriginOGGWriter *oggwriter.OggWriter
}

func (writer *WrappedOGGWriter) WriteRTP(header *rtp.Header, payload []byte) (int, error) {
	return 0, writer.OriginOGGWriter.WriteRTP(&rtp.Packet{
		Header:  *header,
		Payload: payload,
	})
}

func (writer *WrappedOGGWriter) Write(b []byte) (int, error) {
	panic("not implemented")
}

func (track *TrackHandle) WriteTo(duration time.Duration, ssrc uint32, payloadType uint8, writer *oggwriter.OggWriter) {
	// refer to [rfc7587](https://datatracker.ietf.org/doc/html/rfc7587)
	// for the browser support of codecs, refer to https://developer.mozilla.org/en-US/docs/Web/Media/Guides/Formats/WebRTC_codecs

	// sequenceNumber increases by every single packet sent, it's also the sequence number in RTP packet header
	// by RFC 3550, they should be started at random
	var sequenceNumber uint16 = uint16(rand.Uint32())

	// timestamp is the timestamp in RTP packet header,
	// "reflects the sampling instant of the first octet in the RTP data packet."
	// As an example, for fixed-rate audio
	// the timestamp clock would likely increment by one for each
	// sampling period.  If an audio application reads blocks covering
	// 160 sampling periods from the input device, the timestamp would be
	// increased by 160 for each such block, regardless of whether the
	//  block is transmitted in a packet or dropped as silent.
	// // by RFC 3550, they should be started at random
	var timestamp uint32 = rand.Uint32()

	totalSamples := int(duration.Seconds() * float64(track.sampleRate))
	totalPackets := int(float64(totalSamples) / float64(track.samplesPerPacket))

	rtpWriter := &WrappedOGGWriter{
		OriginOGGWriter: writer,
	}

	// pre-populating packets to the receiver's buffer
	var err error
	for i := 0; i < totalPackets; i++ {
		sequenceNumber, timestamp, err = track.encodeAndSend(ssrc, payloadType, rtpWriter, sequenceNumber, timestamp)
		if err != nil {
			log.Println("Failed to encode and send RTP packet", err)
			return
		}
	}
}

// Bind should implement the way how the media data flows from the Track to the PeerConnection
// This will be called internally after signaling is complete and the list of available
// codecs has been determined
func (track *TrackHandle) Bind(ctx webrtc.TrackLocalContext) (webrtc.RTPCodecParameters, error) {
	log.Printf("[track] stream %s is started", track.streamId)
	codecParam := getOpusCodecParams(ctx)
	if codecParam == nil {
		return webrtc.RTPCodecParameters{}, errors.New("no supported codec found, currently only opus is supported")
	}

	go track.startStreaming(ctx, *codecParam)

	return *codecParam, nil
}

// Unbind should implement the teardown logic when the track is no longer needed. This happens
// because a track has been stopped.
func (track *TrackHandle) Unbind(ctx webrtc.TrackLocalContext) error {
	log.Printf("[track] stream %s is tearing down", track.streamId)
	close(track.stopChan)
	return nil
}

// ID is the unique identifier for this Track. This should be unique for the
// stream, but doesn't have to globally unique. A common example would be 'audio' or 'video'
// and StreamID would be 'desktop' or 'webcam'
// Interpretate this as Track ID
func (track *TrackHandle) ID() string {
	return track.trackId
}

// RID is the RTP Stream ID for this track.
// This is the RTP simulcast ID, to distinguish streams with
// different qualities or resolutions.
// We are not doing simulcast, so leave it empty here.
func (track *TrackHandle) RID() string {
	return ""
}

// StreamID is the group this track belongs too. This must be unique
// Better interpretate this as a Group ID, streams are grouped into groups
func (track *TrackHandle) StreamID() string {
	return track.streamId
}

// Kind controls if this TrackLocal is audio or video
func (track *TrackHandle) Kind() webrtc.RTPCodecType {
	return webrtc.RTPCodecTypeAudio
}
