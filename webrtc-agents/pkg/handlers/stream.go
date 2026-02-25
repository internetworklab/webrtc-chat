package handlers

import (
	"encoding/binary"
)

type MessagePatchOrderKind int

const (
	MessagePatchOrderKindAppend MessagePatchOrderKind = iota
	MessagePatchOrderKindReplace
)

type MessagePatchOrder struct {
	// dynamic length, type string
	MessageID string
	Kind      MessagePatchOrderKind
	Value     string
}

// Marshal a MessagePatchOrder object as TLV binary packet
// structure:
// Kind (1 byte, big endian)
// MessageID Length (2 bytes, big endian)
// Message ID (variable length)
// Value Length (2 bytes, big endian)
// Value (variable length)
func (msgPatchOrder *MessagePatchOrder) Marshal() ([]byte, error) {
	// Calculate total size needed
	// 1 byte for Kind
	// 2 bytes for MessageID length
	// len(MessageID) bytes for MessageID (len returns byte length, not rune count)
	// 2 bytes for Value length
	// len(Value) bytes for Value (len returns byte length, not rune count)
	totalSize := 1 + 2 + len(msgPatchOrder.MessageID) + 2 + len(msgPatchOrder.Value)

	buf := make([]byte, totalSize)
	offset := 0

	// Write Kind (1 byte)
	buf[offset] = byte(msgPatchOrder.Kind)
	offset++

	// Write MessageID Length (2 bytes, big endian)
	// Note: len() returns byte length, not number of characters
	binary.BigEndian.PutUint16(buf[offset:], uint16(len(msgPatchOrder.MessageID)))
	offset += 2

	// Write MessageID (variable length)
	copy(buf[offset:], msgPatchOrder.MessageID)
	offset += len(msgPatchOrder.MessageID)

	// Write Value Length (2 bytes, big endian)
	// Note: len() returns byte length, not number of characters
	binary.BigEndian.PutUint16(buf[offset:], uint16(len(msgPatchOrder.Value)))
	offset += 2

	// Write Value (variable length)
	copy(buf[offset:], msgPatchOrder.Value)

	return buf, nil
}
