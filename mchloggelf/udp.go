package mchloggelf

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"fmt"
	"net"
	"sync"
)

const (
	// maxChunkSize is the maximum size of a single UDP datagram for GELF.
	maxChunkSize = 8192
	// chunkHeaderSize is the size of the GELF chunk header (magic + msgID + seqNum + seqCount).
	chunkHeaderSize = 12
	// maxChunkDataSize is the maximum payload per chunk.
	maxChunkDataSize = maxChunkSize - chunkHeaderSize
	// maxChunks is the maximum number of chunks per GELF message.
	maxChunks = 128
	// chunkMagicByte0 and chunkMagicByte1 are the GELF chunk magic bytes.
	chunkMagicByte0 = 0x1e
	chunkMagicByte1 = 0x0f
)

// UDPTransport sends GELF messages over UDP.
type UDPTransport struct {
	addr     *net.UDPAddr
	conn     *net.UDPConn
	compress bool
	mu       sync.Mutex
}

// NewUDPTransport creates a new UDP transport that sends GELF messages to the given address.
// The address should be in "host:port" format (e.g., "graylog.example.com:12201").
// If compress is true, messages will be GZIP compressed before sending.
func NewUDPTransport(address string, compress bool) (*UDPTransport, error) {
	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve UDP address %q: %w", address, err)
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return nil, fmt.Errorf("failed to dial UDP %q: %w", address, err)
	}

	return &UDPTransport{
		addr:     addr,
		conn:     conn,
		compress: compress,
	}, nil
}

// Send marshals the GELF message and sends it over UDP.
// If the message exceeds maxChunkSize, it is split into GELF chunks.
func (t *UDPTransport) Send(msg *GELFMessage) error {
	data, err := msg.MarshalJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal GELF message: %w", err)
	}

	if t.compress {
		data, err = gzipCompress(data)
		if err != nil {
			return fmt.Errorf("failed to compress GELF message: %w", err)
		}
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.conn == nil {
		return fmt.Errorf("transport is closed")
	}

	if len(data) <= maxChunkSize {
		_, err = t.conn.Write(data)
		return err
	}

	return t.sendChunked(data)
}

// sendChunked splits the data into GELF chunks and sends each one.
func (t *UDPTransport) sendChunked(data []byte) error {
	chunkCount := (len(data) + maxChunkDataSize - 1) / maxChunkDataSize
	if chunkCount > maxChunks {
		return fmt.Errorf("message too large: would require %d chunks (max %d)", chunkCount, maxChunks)
	}

	// Generate a random 8-byte message ID
	msgID := make([]byte, 8)
	if _, err := rand.Read(msgID); err != nil {
		return fmt.Errorf("failed to generate message ID: %w", err)
	}

	for i := 0; i < chunkCount; i++ {
		start := i * maxChunkDataSize
		end := start + maxChunkDataSize
		if end > len(data) {
			end = len(data)
		}

		chunk := make([]byte, 0, chunkHeaderSize+end-start)
		// Magic bytes
		chunk = append(chunk, chunkMagicByte0, chunkMagicByte1)
		// Message ID (8 bytes)
		chunk = append(chunk, msgID...)
		// Sequence number (1 byte)
		chunk = append(chunk, byte(i))
		// Sequence count (1 byte)
		chunk = append(chunk, byte(chunkCount))
		// Payload
		chunk = append(chunk, data[start:end]...)

		if _, err := t.conn.Write(chunk); err != nil {
			return fmt.Errorf("failed to send chunk %d/%d (chunks 1-%d already sent, message will be incomplete on receiver): %w", i+1, chunkCount, i, err)
		}
	}

	return nil
}

// Close closes the UDP connection.
func (t *UDPTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn != nil {
		err := t.conn.Close()
		t.conn = nil
		return err
	}
	return nil
}

// gzipCompress compresses data using GZIP.
func gzipCompress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
