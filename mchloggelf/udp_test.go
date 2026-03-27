package mchloggelf

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net"
	"testing"
	"time"
)

func TestUDPTransportSendSmallMessage(t *testing.T) {
	// Start a local UDP listener
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ResolveUDPAddr: %v", err)
	}
	listener, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatalf("ListenUDP: %v", err)
	}
	defer listener.Close()

	localAddr := listener.LocalAddr().String()

	// Create transport without compression for easier validation
	transport, err := NewUDPTransport(localAddr, false)
	if err != nil {
		t.Fatalf("NewUDPTransport: %v", err)
	}
	defer transport.Close()

	msg := &GELFMessage{
		Version:      "1.1",
		Host:         "testhost",
		ShortMessage: "hello gelf",
		Timestamp:    1234567890.123,
		Level:        SyslogInformational,
		Extra:        map[string]any{"service": "test-svc"},
	}

	if err := transport.Send(msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Read from listener
	buf := make([]byte, maxChunkSize)
	listener.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err := listener.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("ReadFromUDP: %v", err)
	}

	var received map[string]any
	if err := json.Unmarshal(buf[:n], &received); err != nil {
		t.Fatalf("Unmarshal received data: %v", err)
	}

	if received["version"] != "1.1" {
		t.Errorf("version = %v, want %q", received["version"], "1.1")
	}
	if received["short_message"] != "hello gelf" {
		t.Errorf("short_message = %v, want %q", received["short_message"], "hello gelf")
	}
	if received["_service"] != "test-svc" {
		t.Errorf("_service = %v, want %q", received["_service"], "test-svc")
	}
}

func TestUDPTransportSendWithCompression(t *testing.T) {
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ResolveUDPAddr: %v", err)
	}
	listener, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatalf("ListenUDP: %v", err)
	}
	defer listener.Close()

	localAddr := listener.LocalAddr().String()

	transport, err := NewUDPTransport(localAddr, true)
	if err != nil {
		t.Fatalf("NewUDPTransport: %v", err)
	}
	defer transport.Close()

	msg := &GELFMessage{
		Version:      "1.1",
		Host:         "testhost",
		ShortMessage: "compressed message",
		Timestamp:    1234567890.123,
		Level:        SyslogDebug,
		Extra:        map[string]any{},
	}

	if err := transport.Send(msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	buf := make([]byte, maxChunkSize)
	listener.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err := listener.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("ReadFromUDP: %v", err)
	}

	// Data should be gzip compressed
	reader, err := gzip.NewReader(bytes.NewReader(buf[:n]))
	if err != nil {
		t.Fatalf("gzip.NewReader: %v (data might not be compressed)", err)
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	var received map[string]any
	if err := json.Unmarshal(decompressed, &received); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if received["short_message"] != "compressed message" {
		t.Errorf("short_message = %v, want %q", received["short_message"], "compressed message")
	}
}

func TestUDPTransportChunking(t *testing.T) {
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ResolveUDPAddr: %v", err)
	}
	listener, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatalf("ListenUDP: %v", err)
	}
	defer listener.Close()

	localAddr := listener.LocalAddr().String()

	// Create transport without compression so the message stays large
	transport, err := NewUDPTransport(localAddr, false)
	if err != nil {
		t.Fatalf("NewUDPTransport: %v", err)
	}
	defer transport.Close()

	// Create a large message that will require chunking
	largePayload := make([]byte, 10000)
	for i := range largePayload {
		largePayload[i] = 'A'
	}

	msg := &GELFMessage{
		Version:      "1.1",
		Host:         "testhost",
		ShortMessage: string(largePayload),
		Timestamp:    1234567890.123,
		Level:        SyslogInformational,
		Extra:        map[string]any{},
	}

	if err := transport.Send(msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Read chunks
	listener.SetReadDeadline(time.Now().Add(2 * time.Second))
	chunks := make([][]byte, 0)
	for {
		buf := make([]byte, maxChunkSize+100)
		n, _, err := listener.ReadFromUDP(buf)
		if err != nil {
			break
		}
		chunks = append(chunks, buf[:n])
	}

	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}

	// Verify chunk headers
	for i, chunk := range chunks {
		if len(chunk) < chunkHeaderSize {
			t.Fatalf("chunk %d too small: %d bytes", i, len(chunk))
		}
		if chunk[0] != chunkMagicByte0 || chunk[1] != chunkMagicByte1 {
			t.Errorf("chunk %d: invalid magic bytes: %x %x", i, chunk[0], chunk[1])
		}
		seqNum := int(chunk[10])
		seqCount := int(chunk[11])
		if seqNum != i {
			t.Errorf("chunk %d: sequence number = %d, want %d", i, seqNum, i)
		}
		if seqCount != len(chunks) {
			t.Errorf("chunk %d: sequence count = %d, want %d", i, seqCount, len(chunks))
		}
	}

	// All chunks should share the same message ID (bytes 2-9)
	if len(chunks) > 1 {
		msgID := chunks[0][2:10]
		for i := 1; i < len(chunks); i++ {
			if !bytes.Equal(chunks[i][2:10], msgID) {
				t.Errorf("chunk %d has different message ID", i)
			}
		}
	}
}

func TestGzipCompress(t *testing.T) {
	input := []byte("hello world, this is a test of gzip compression")

	compressed, err := gzipCompress(input)
	if err != nil {
		t.Fatalf("gzipCompress: %v", err)
	}

	// Verify we can decompress
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if !bytes.Equal(decompressed, input) {
		t.Errorf("decompressed data does not match original")
	}
}

func TestUDPTransportClose(t *testing.T) {
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ResolveUDPAddr: %v", err)
	}
	listener, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatalf("ListenUDP: %v", err)
	}
	defer listener.Close()

	transport, err := NewUDPTransport(listener.LocalAddr().String(), false)
	if err != nil {
		t.Fatalf("NewUDPTransport: %v", err)
	}

	if err := transport.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}

	// Sending after close should fail
	msg := &GELFMessage{
		Version:      "1.1",
		Host:         "testhost",
		ShortMessage: "after close",
		Timestamp:    1234567890.123,
		Level:        SyslogInformational,
		Extra:        map[string]any{},
	}

	if err := transport.Send(msg); err == nil {
		t.Error("expected error when sending after close")
	}
}

func TestNewUDPTransportInvalidAddress(t *testing.T) {
	_, err := NewUDPTransport("invalid:::address", false)
	if err == nil {
		t.Error("expected error for invalid address")
	}
}
