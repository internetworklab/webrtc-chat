package main

import (
	"log"
	"time"

	pkgtracks "webrtc-agents/pkg/tracks"
	pkgwn "webrtc-agents/pkg/tracks/wn"

	"github.com/google/uuid"
	"github.com/hraban/opus"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media/oggwriter"
)

func main() {
	frameIntv := pkgtracks.DefaultFrameIntv
	sampleRate := pkgtracks.DefaultSampleRate
	channels := pkgtracks.DefaultChannelsCount
	samplesPerPacket := int(float64(frameIntv.Seconds()) * float64(sampleRate))
	var ssrc uint32 = 1
	codecUsed := webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{webrtc.MimeTypeOpus, 48000, 2, "minptime=10;useinbandfec=1", nil},
		PayloadType:        111,
	}

	enc, err := opus.NewEncoder(
		sampleRate,
		channels,
		opus.AppAudio,
	)
	if err != nil {
		log.Fatalf("failed to create encoder: %v", err)
	}

	packetGen, err := pkgwn.NewOpusWhiteNoiseGenerator(
		enc, channels, samplesPerPacket, pkgtracks.DefaultMaxPayloadSize,
	)
	if err != nil {
		log.Fatalf("failed to create white noise RTP packet generator: %v", err)
	}

	// Create a new audio track
	track, err := pkgtracks.NewTrackHandle(uuid.New().String(), frameIntv, sampleRate, channels, packetGen)
	if err != nil {
		log.Fatalf("failed to create track: %v", err)
	}

	writer, err := oggwriter.New("test.ogg", uint32(sampleRate), uint16(channels))
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
	track.WriteTo(songDuration, ssrc, uint8(codecUsed.PayloadType), writer)
	log.Println("Done")
}
