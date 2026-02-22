package oggfile

import (
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4/pkg/media/oggreader"
)

// OggFileGenerator reads an OGG file and generates RTP packets from it.
// The OGG file must contain Opus-encoded audio.
type OggFileGenerator struct {
	name       string
	filePath   string
	reader     *oggreader.OggReader
	header     *oggreader.OggHeader
	file       *os.File
	mu         sync.Mutex
	granulePos uint64
	done       bool
}

// NewOggFileGenerator creates a new OGG file-based generator.
// The OGG file must contain Opus-encoded audio.
func NewOggFileGenerator(name, filePath string) (*OggFileGenerator, error) {
	gen := &OggFileGenerator{
		name:     name,
		filePath: filePath,
	}

	if err := gen.resetReader(); err != nil {
		return nil, err
	}

	return gen, nil
}

// resetReader opens the file and initializes the OGG reader
func (g *OggFileGenerator) resetReader() error {
	file, err := os.Open(g.filePath)
	if err != nil {
		return fmt.Errorf("failed to open ogg file %s: %w", g.filePath, err)
	}

	reader, header, err := oggreader.NewWith(file)
	if err != nil {
		file.Close()
		return fmt.Errorf("failed to create ogg reader for %s: %w", g.filePath, err)
	}

	g.file = file
	g.reader = reader
	g.header = header
	g.granulePos = 0
	g.done = false

	return nil
}

// GetPacket returns the next RTP packet from the OGG file.
// It reads the next page from the OGG container and wraps the Opus payload in an RTP packet.
// Returns io.EOF when the file has been fully read.
func (g *OggFileGenerator) GetPacket(ssrc uint32, payloadType uint8, sequenceNumber uint16, timestamp uint32) (*rtp.Packet, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.done {
		return nil, io.EOF
	}

	// Read the next page from the OGG file
	payload, pageHeader, err := g.reader.ParseNextPage()
	if err != nil {
		if err == io.EOF {
			g.done = true
		}
		return nil, err
	}

	// Skip metadata pages (OpusHead and OpusTags) - they don't contain audio data
	headerType, isOpus := pageHeader.HeaderType(payload)
	if isOpus && (headerType == oggreader.HeaderOpusID || headerType == oggreader.HeaderOpusTags) {
		// Recursively get the next packet
		return g.GetPacket(ssrc, payloadType, sequenceNumber, timestamp)
	}

	// Update granule position for timing tracking
	if pageHeader.GranulePosition != 0 {
		g.granulePos = pageHeader.GranulePosition
	}

	// Create RTP packet with the Opus payload
	// The payload from OGG is already Opus-encoded, no need to re-encode
	packet := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    payloadType,
			SequenceNumber: sequenceNumber,
			Timestamp:      timestamp,
			SSRC:           ssrc,
		},
		Payload: payload,
	}

	return packet, nil
}

// GetName returns the name of this generator
func (g *OggFileGenerator) GetName() string {
	return g.name
}

// GetSampleRate returns the sample rate of the OGG file
func (g *OggFileGenerator) GetSampleRate() uint32 {
	if g.header == nil {
		return 0
	}
	return g.header.SampleRate
}

// GetChannels returns the number of channels in the OGG file
func (g *OggFileGenerator) GetChannels() uint8 {
	if g.header == nil {
		return 0
	}
	return g.header.Channels
}

// GetDuration returns the approximate duration in samples based on granule position
func (g *OggFileGenerator) GetDuration() uint64 {
	return g.granulePos
}

// IsDone returns true if the file has been fully read
func (g *OggFileGenerator) IsDone() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.done
}

// Reset resets the reader to the beginning of the file for looping playback
func (g *OggFileGenerator) Reset() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Close current file
	if g.file != nil {
		g.file.Close()
	}

	return g.resetReader()
}

// Close closes the underlying file and releases resources
func (g *OggFileGenerator) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.file != nil {
		err := g.file.Close()
		g.file = nil
		return err
	}
	return nil
}

// Looper wraps an OggFileGenerator to provide infinite looping playback.
// When the file ends, it automatically resets to the beginning.
type Looper struct {
	gen *OggFileGenerator
	mu  sync.Mutex
}

// NewLooper creates a new looping OGG player
func NewLooper(gen *OggFileGenerator) *Looper {
	return &Looper{gen: gen}
}

// GetPacket returns the next RTP packet, looping back to the start when EOF is reached
func (l *Looper) GetPacket(ssrc uint32, payloadType uint8, sequenceNumber uint16, timestamp uint32) (*rtp.Packet, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	pkt, err := l.gen.GetPacket(ssrc, payloadType, sequenceNumber, timestamp)
	if err == io.EOF {
		// Reset and try again
		if resetErr := l.gen.resetReader(); resetErr != nil {
			return nil, resetErr
		}
		// Recursively get the next packet (will skip headers)
		return l.gen.GetPacket(ssrc, payloadType, sequenceNumber, timestamp)
	}
	return pkt, err
}

// GetName returns the name of the underlying generator
func (l *Looper) GetName() string {
	return l.gen.GetName()
}

// Close closes the underlying generator
func (l *Looper) Close() error {
	return l.gen.Close()
}
