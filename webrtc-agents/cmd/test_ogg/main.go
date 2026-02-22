package main

import (
	"fmt"
	"log"
	"time"

	pkgwn "webrtc-agents/pkg/tracks/wn"

	"github.com/google/uuid"
	"github.com/hraban/opus"
	webrtc "github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media/oggwriter"
)

// MyWHTrack implements a TrackLocal interface
type MyWHTrack struct {
	trackId          string
	streamId         string
	stopChan         chan struct{}
	sampleRate       int
	numChannels      int
	mtu              int
	opusEncoder      *opus.Encoder
	maxPayloadSize   int
	frameIntv        time.Duration
	samplesPerPacket int
	whGen            *pkgwn.OpusWhiteNoiseGenerator
}

func NewMyWHTrack(streamId string) (*MyWHTrack, error) {

	wh := &MyWHTrack{
		trackId:     fmt.Sprintf("audio-%s", uuid.New().String()),
		streamId:    streamId,
		stopChan:    make(chan struct{}),
		sampleRate:  48000,
		numChannels: 2,
		mtu:         1280, // 1280 is the minimum requirement for IPv6 over ethernet
		frameIntv:   20 * time.Millisecond,
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

func (track *MyWHTrack) encodeAndSend(ssrc uint32, selectedCodec webrtc.RTPCodecParameters, writer *oggwriter.OggWriter, sequenceNumber uint16, timestamp uint32) (uint16, uint32, error) {
	pkt, err := track.whGen.GetPacket(ssrc, uint8(selectedCodec.PayloadType), sequenceNumber, timestamp)
	if err != nil {
		return sequenceNumber, timestamp, err
	}

	if err := writer.WriteRTP(pkt); err != nil {
		return sequenceNumber, timestamp, err
	}

	sequenceNumber++
	timestamp += uint32(track.samplesPerPacket)
	return sequenceNumber, timestamp, nil
}

func (track *MyWHTrack) StartWriting(duration time.Duration, ssrc uint32, selectedCodec webrtc.RTPCodecParameters, writer *oggwriter.OggWriter) {

}

func main() {
	track, err := NewMyWHTrack(uuid.New().String())
	if err != nil {
		log.Fatalln("Failed to create track:", err)
	}
	log.Println("Track created")
	sampleRate := 48000
	nChannels := 2
	writer, err := oggwriter.New("test.ogg", uint32(sampleRate), uint16(nChannels))
	if err != nil {
		log.Fatalln("Failed to create ogg writer:", err)
	}
	log.Println("Ogg writer created")
	defer writer.Close()

	songDuration, err := time.ParseDuration("3s")
	if err != nil {
		panic(err)
	}

	log.Println("Start writing")
	track.StartWriting(songDuration, 1, webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{webrtc.MimeTypeOpus, 48000, 2, "minptime=10;useinbandfec=1", nil},
		PayloadType:        111,
	}, writer)
	log.Println("Done")
}
