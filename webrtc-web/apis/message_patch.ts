import { MessagePatchOrder, MessagePatchOrderKind } from "./types";

export function unmarshalMessagePatchOrder(
  data: ArrayBuffer,
): MessagePatchOrder | undefined {
  const view = new DataView(data);
  let offset = 0;

  // Read Kind (1 byte)
  const kind = view.getUint8(offset);
  offset += 1;

  // Read MessageID Length (2 bytes, big endian)
  const messageIdLength = view.getUint16(offset, false); // false = big endian
  offset += 2;

  // Check if buffer has enough data for MessageID
  if (data.byteLength < offset + messageIdLength) {
    throw new Error(
      "Invalid message patch order: insufficient data for MessageID",
    );
  }

  // Read MessageID (variable length)
  const messageIdDecoder = new TextDecoder();
  const messageIdBytes = new Uint8Array(data, offset, messageIdLength);
  const messageId = messageIdDecoder.decode(messageIdBytes);
  offset += messageIdLength;

  // Read Value Length (2 bytes, big endian)
  const valueLength = view.getUint16(offset, false); // false = big endian
  offset += 2;

  // Check if buffer has enough data for Value
  if (data.byteLength < offset + valueLength) {
    throw new Error("Invalid message patch order: insufficient data for Value");
  }

  // Read Value (variable length)
  const valueBytes = new Uint8Array(data, offset, valueLength);
  const value = messageIdDecoder.decode(valueBytes);

  return {
    MessageID: messageId,
    Kind: kind as MessagePatchOrderKind,
    Value: value,
  };
}
