package tracks

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"time"

	pkgwn "webrtc-agents/pkg/tracks/wn"

	"github.com/google/uuid"
	"github.com/hraban/opus"
	webrtc "github.com/pion/webrtc/v4"
)

// MyWHTrack implements a TrackLocal interface
type MyWHTrack struct {
	trackId               string
	streamId              string
	stopChan              chan struct{}
	sampleRate            int
	numChannels           int
	mtu                   int
	opusEncoder           *opus.Encoder
	maxPayloadSize        int
	frameIntv             time.Duration
	samplesPerPacket      int
	numPrePopulatePackets int
	whGen                 *pkgwn.OpusWhiteNoiseGenerator
}

func NewMyWHTrack(streamId string) (*MyWHTrack, error) {

	wh := &MyWHTrack{
		trackId:               fmt.Sprintf("audio-%s", uuid.New().String()),
		streamId:              streamId,
		stopChan:              make(chan struct{}),
		sampleRate:            48000,
		numChannels:           2,
		mtu:                   1280, // 1280 is the minimum requirement for IPv6 over ethernet
		frameIntv:             20 * time.Millisecond,
		numPrePopulatePackets: 10,
	}

	// The Opus encoder can output encoded frames representing 2.5, 5, 10,
	// 20, 40, or 60 ms of speech or audio data.
	// we would use 20ms here, which would be (20ms/1000ms)*(48000samples/s)=960 samples per packet

	wh.samplesPerPacket = int(float64(wh.frameIntv.Milliseconds()) / float64(1000) * float64(wh.sampleRate))

	wh.maxPayloadSize = wh.mtu -
		40 - // IPv6 header
		8 - // UDP header
		16 - // SRTP Auth Tag, would be 10 bytes (for HMAC-SHA1), or 16 bytes (for AES-GCM)
		12 - // minimum RTP header of fixed size
		20 // possible RTP extensions

	enc, err := opus.NewEncoder(wh.sampleRate, wh.numChannels, opus.AppVoIP)
	if err != nil {
		return nil, fmt.Errorf("failed to create opus encoder: %w", err)
	}
	wh.opusEncoder = enc

	wh.whGen, err = pkgwn.NewOpusWhiteNoiseGenerator(
		enc,
		wh.numChannels,
		wh.samplesPerPacket,
		wh.maxPayloadSize,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize packet generator")
	}

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

func (track *MyWHTrack) encodeAndSend(ctx webrtc.TrackLocalContext, selectedCodec webrtc.RTPCodecParameters, sequenceNumber uint16, timestamp uint32) (uint16, uint32, error) {
	pkt, err := track.whGen.GetPacket(uint32(ctx.SSRC()), uint8(selectedCodec.PayloadType), sequenceNumber, timestamp)
	if err != nil {
		return sequenceNumber, timestamp, err
	}

	if _, err := ctx.WriteStream().WriteRTP(&pkt.Header, pkt.Payload); err != nil {
		return sequenceNumber, timestamp, err
	}

	sequenceNumber++
	timestamp += uint32(track.samplesPerPacket)
	return sequenceNumber, timestamp, nil
}

func (track *MyWHTrack) startStreaming(ctx webrtc.TrackLocalContext, selectedCodec webrtc.RTPCodecParameters) {
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

	// pre-populating packets to the receiver's buffer
	var err error
	for i := 0; i < track.numPrePopulatePackets; i++ {
		sequenceNumber, timestamp, err = track.encodeAndSend(ctx, selectedCodec, sequenceNumber, timestamp)
		if err != nil {
			log.Println("Failed to encode and send RTP packet")
			return
		}
	}

	ticker := time.NewTicker(track.frameIntv)

	for {
		select {
		case <-ticker.C:

			sequenceNumber, timestamp, err = track.encodeAndSend(ctx, selectedCodec, sequenceNumber, timestamp)
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

// Bind should implement the way how the media data flows from the Track to the PeerConnection
// This will be called internally after signaling is complete and the list of available
// codecs has been determined
func (track *MyWHTrack) Bind(ctx webrtc.TrackLocalContext) (webrtc.RTPCodecParameters, error) {
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
func (track *MyWHTrack) Unbind(ctx webrtc.TrackLocalContext) error {
	log.Printf("[track] stream %s is tearing down", track.streamId)
	close(track.stopChan)
	return nil
}

// ID is the unique identifier for this Track. This should be unique for the
// stream, but doesn't have to globally unique. A common example would be 'audio' or 'video'
// and StreamID would be 'desktop' or 'webcam'
// Interpretate this as Track ID
func (track *MyWHTrack) ID() string {
	return track.trackId
}

// RID is the RTP Stream ID for this track.
// This is the RTP simulcast ID, to distinguish streams with
// different qualities or resolutions.
// We are not doing simulcast, so leave it empty here.
func (track *MyWHTrack) RID() string {
	return ""
}

// StreamID is the group this track belongs too. This must be unique
// Better interpretate this as a Group ID, streams are grouped into groups
func (track *MyWHTrack) StreamID() string {
	return track.streamId
}

// Kind controls if this TrackLocal is audio or video
func (track *MyWHTrack) Kind() webrtc.RTPCodecType {
	return webrtc.RTPCodecTypeAudio
}
