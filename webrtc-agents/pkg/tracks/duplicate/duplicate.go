package duplicate

import (
	"log"
	"os"
	"sync"

	"github.com/pion/interceptor"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

// DefaultBufferSize is the number of RTP packets to buffer per consumer
const DefaultBufferSize = 100

// rtpPacket represents an RTP packet with header and payload for channel passing
type rtpPacket struct {
	header  *rtp.Header
	payload []byte
}

// TrackDuplicater implements webrtc.TrackLocal interface and allows
// one upstream track to be duplicated to multiple downstream consumers.
// It binds the upstream TrackLocal with a custom context that intercepts
// RTP packets and forwards them to all bound downstream consumers.
type TrackDuplicater struct {
	Upstream webrtc.TrackLocal

	downstreams   sync.Map       // downstream context ID -> *downstreamConsumer
	mu            sync.Mutex     // protects sourceContext and codec
	sourceContext *sourceContext // custom context for upstream binding
	codec         webrtc.RTPCodecParameters
	logger        *log.Logger
	bufferSize    int // buffer size per consumer
}

// downstreamConsumer wraps a TrackLocalContext with a buffered channel for async writes
type downstreamConsumer struct {
	ctx     webrtc.TrackLocalContext
	packets chan *rtpPacket
	stop    chan struct{}
	wg      sync.WaitGroup
}

// sourceContext implements webrtc.TrackLocalContext to intercept writes from upstream
type sourceContext struct {
	duplicater *TrackDuplicater
	id         string
	ssrc       webrtc.SSRC
	codec      webrtc.RTPCodecParameters
	writer     *forwardingWriter
}

// forwardingWriter implements webrtc.TrackLocalWriter to capture and forward RTP packets
type forwardingWriter struct {
	duplicater *TrackDuplicater
}

// NewTrackDuplicater creates a new TrackDuplicater that wraps an upstream TrackLocal
func NewTrackDuplicater(upstream webrtc.TrackLocal) *TrackDuplicater {
	return &TrackDuplicater{
		Upstream:   upstream,
		logger:     log.New(os.Stderr, "[TrackDuplicater] ", log.LstdFlags|log.Lmsgprefix),
		bufferSize: DefaultBufferSize,
	}
}

// NewTrackDuplicaterWithBufferSize creates a new TrackDuplicater with a custom buffer size
func NewTrackDuplicaterWithBufferSize(upstream webrtc.TrackLocal, bufferSize int) *TrackDuplicater {
	return &TrackDuplicater{
		Upstream:   upstream,
		logger:     log.New(os.Stderr, "[TrackDuplicater] ", log.LstdFlags|log.Lmsgprefix),
		bufferSize: bufferSize,
	}
}

// startWriter starts a goroutine that reads from the packet channel and writes to the consumer
func (dc *downstreamConsumer) startWriter(logger *log.Logger) {
	dc.wg.Add(1)
	go func() {
		defer dc.wg.Done()
		writer := dc.ctx.WriteStream()
		for {
			select {
			case pkt := <-dc.packets:
				if _, err := writer.WriteRTP(pkt.header, pkt.payload); err != nil {
					logger.Printf("Failed to write RTP to downstream %s: %v", dc.ctx.ID(), err)
				}
			case <-dc.stop:
				// Drain remaining packets before stopping
				for {
					select {
					case pkt := <-dc.packets:
						if _, err := writer.WriteRTP(pkt.header, pkt.payload); err != nil {
							logger.Printf("Failed to write RTP to downstream %s: %v", dc.ctx.ID(), err)
						}
					default:
						return
					}
				}
			}
		}
	}()
}

// close stops the writer goroutine and waits for it to finish
func (dc *downstreamConsumer) close() {
	close(dc.stop)
	dc.wg.Wait()
}

// send sends an RTP packet to the consumer's buffer (non-blocking, drops if full)
func (dc *downstreamConsumer) send(pkt *rtpPacket) bool {
	select {
	case dc.packets <- pkt:
		return true
	default:
		return false // buffer full, packet dropped
	}
}

// Bind is called when this track is added to a PeerConnection.
// It ensures the upstream is bound and stores the downstream context for forwarding.
func (dup *TrackDuplicater) Bind(ctx webrtc.TrackLocalContext) (webrtc.RTPCodecParameters, error) {
	// Create a downstream consumer with buffered channel
	consumer := &downstreamConsumer{
		ctx:     ctx,
		packets: make(chan *rtpPacket, dup.bufferSize),
		stop:    make(chan struct{}),
	}
	consumer.startWriter(dup.logger)

	// Store the downstream consumer (sync.Map is safe for concurrent access)
	dup.downstreams.Store(ctx.ID(), consumer)

	dup.mu.Lock()
	defer dup.mu.Unlock()

	dup.logger.Printf("Binding downstream, ctx id: %s, track: %s", ctx.ID(), dup.ID())

	// If this is the first downstream, bind the upstream
	if dup.sourceContext == nil {
		// Create a source context for the upstream
		codec := webrtc.RTPCodecParameters{}
		if codecs := ctx.CodecParameters(); len(codecs) > 0 {
			codec = codecs[0]
		}

		dup.sourceContext = &sourceContext{
			duplicater: dup,
			id:         "source-" + dup.ID(),
			ssrc:       ctx.SSRC(), // Use the same SSRC or generate a new one
			codec:      codec,
			writer: &forwardingWriter{
				duplicater: dup,
			},
		}

		// Bind the upstream with our custom context
		upstreamCodec, err := dup.Upstream.Bind(dup.sourceContext)
		if err != nil {
			dup.logger.Printf("Failed to bind upstream: %v", err)
			if c, ok := dup.downstreams.Load(ctx.ID()); ok {
				c.(*downstreamConsumer).close()
			}
			dup.downstreams.Delete(ctx.ID())
			dup.sourceContext = nil
			return webrtc.RTPCodecParameters{}, err
		}

		dup.codec = upstreamCodec
		dup.logger.Printf("Upstream bound successfully with codec: %s", upstreamCodec.MimeType)
	} else {
		// Upstream already bound, use the existing codec
		dup.codec = dup.sourceContext.codec
	}

	return dup.codec, nil
}

// Unbind is called when the track is removed from a PeerConnection.
// It removes the downstream context and unbinds the upstream if no more downstreams.
func (dup *TrackDuplicater) Unbind(ctx webrtc.TrackLocalContext) error {
	// Stop the consumer goroutine and remove from downstreams
	if c, ok := dup.downstreams.Load(ctx.ID()); ok {
		consumer := c.(*downstreamConsumer)
		consumer.close()
		dup.downstreams.Delete(ctx.ID())
	}

	dup.mu.Lock()
	defer dup.mu.Unlock()

	dup.logger.Printf("Unbinding downstream, ctx id: %s", ctx.ID())

	// Check if no more downstreams remain
	if dup.consumerCount() == 0 && dup.sourceContext != nil {
		dup.logger.Printf("No more downstreams, unbinding upstream")
		if err := dup.Upstream.Unbind(dup.sourceContext); err != nil {
			dup.logger.Printf("Failed to unbind upstream: %v", err)
		}
		dup.sourceContext = nil
	}

	return nil
}

// ID returns the unique identifier for this Track.
func (dup *TrackDuplicater) ID() string {
	if dup.Upstream != nil {
		return dup.Upstream.ID()
	}
	return ""
}

// RID returns the RTP Stream ID for this track.
func (dup *TrackDuplicater) RID() string {
	if dup.Upstream != nil {
		return dup.Upstream.RID()
	}
	return ""
}

// StreamID returns the group this track belongs to.
func (dup *TrackDuplicater) StreamID() string {
	if dup.Upstream != nil {
		return dup.Upstream.StreamID()
	}
	return ""
}

// Kind returns whether this TrackLocal is audio or video.
func (dup *TrackDuplicater) Kind() webrtc.RTPCodecType {
	if dup.Upstream != nil {
		return dup.Upstream.Kind()
	}
	return webrtc.RTPCodecType(0)
}

// ConsumerCount returns the number of currently bound downstream consumers.
func (dup *TrackDuplicater) ConsumerCount() int {
	return dup.consumerCount()
}

// consumerCount is an internal helper that counts downstreams without locking.
// It must only be called when the caller holds the mutex or from Unbind which handles its own locking.
func (dup *TrackDuplicater) consumerCount() int {
	count := 0
	dup.downstreams.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}

// GetCodec returns the negotiated codec parameters.
func (dup *TrackDuplicater) GetCodec() webrtc.RTPCodecParameters {
	dup.mu.Lock()
	defer dup.mu.Unlock()
	return dup.codec
}

// forwardToDownstreams forwards RTP packets to all downstream consumers asynchronously.
// This is called by the forwardingWriter when the upstream writes data.
// Packets are sent to each consumer's buffer channel; if a buffer is full, the packet is dropped.
func (dup *TrackDuplicater) forwardToDownstreams(header *rtp.Header, payload []byte) {
	pkt := &rtpPacket{header: header, payload: payload}
	dup.downstreams.Range(func(key, value interface{}) bool {
		ctxID := key.(string)
		consumer := value.(*downstreamConsumer)
		if !consumer.send(pkt) {
			dup.logger.Printf("Dropped packet for downstream %s (buffer full)", ctxID)
		}
		return true
	})
}

// --- sourceContext implementation (webrtc.TrackLocalContext) ---

func (sc *sourceContext) ID() string {
	return sc.id
}

func (sc *sourceContext) SSRC() webrtc.SSRC {
	return sc.ssrc
}

func (sc *sourceContext) SSRCForwardErrorCorrection() webrtc.SSRC {
	sc.duplicater.logger.Printf("SSRCForwardErrorCorrection called (not supported)")
	return 0 // We don't use Forward Error Correction
}

func (sc *sourceContext) SSRCRetransmission() webrtc.SSRC {
	sc.duplicater.logger.Printf("SSRCRetransmission called (not supported)")
	return 0 // We don't use RTX retransmission
}

func (sc *sourceContext) CodecParameters() []webrtc.RTPCodecParameters {
	return []webrtc.RTPCodecParameters{sc.codec}
}

func (sc *sourceContext) WriteStream() webrtc.TrackLocalWriter {
	return sc.writer
}

func (sc *sourceContext) HeaderExtensions() []webrtc.RTPHeaderExtensionParameter {
	sc.duplicater.logger.Printf("HeaderExtensions called (not supported)")
	return nil
}

func (sc *sourceContext) RTCPReader() interceptor.RTCPReader {
	sc.duplicater.logger.Printf("RTCPReader called (not supported)")
	return nil // We don't handle RTCP feedback in the duplicator
}

// --- forwardingWriter implementation (webrtc.TrackLocalWriter) ---

func (fw *forwardingWriter) WriteRTP(header *rtp.Header, payload []byte) (int, error) {
	// Forward the packet to all downstream consumers
	fw.duplicater.forwardToDownstreams(header, payload)
	return len(payload), nil
}

func (fw *forwardingWriter) Write(b []byte) (int, error) {
	// Parse as RTP packet and forward
	// This is a fallback for raw byte writes
	var pkt rtp.Packet
	if err := pkt.Unmarshal(b); err != nil {
		fw.duplicater.logger.Printf("Failed to unmarshal RTP packet: %v", err)
		return 0, err
	}
	fw.duplicater.forwardToDownstreams(&pkt.Header, pkt.Payload)
	return len(b), nil
}
