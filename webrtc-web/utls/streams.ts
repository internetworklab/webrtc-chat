export const wordSize = 4;

export function createStreamFromDataChannel(channel: RTCDataChannel) {
  if (channel.binaryType !== "arraybuffer") {
    console.log("setting binary type to arraybuffer for consistency", channel);
    channel.binaryType = "arraybuffer";
  }

  return new ReadableStream({
    start(controller) {
      channel.onmessage = (event) => {
        controller.enqueue(event.data);
      };
    },

    cancel() {
      // we don't close the underlying data channel here,
      // it will be closed at somewhere else.
    },
  });
}

// parse a network stream in network byte order (big endian) into a stream of 32-bit words
export function newUint32StreamParser() {
  let feedBackParseRef: {
    chunks: any[];
    totalSize: number;
  } = {
    chunks: [],
    totalSize: 0,
  };

  const doConsume = async () => {
    if (feedBackParseRef.totalSize >= wordSize) {
      const mergedChunk = new Blob(feedBackParseRef.chunks);
      const rest = feedBackParseRef.totalSize % wordSize;
      feedBackParseRef.totalSize = 0;
      feedBackParseRef.chunks = [];
      if (rest > 0) {
        const restChunk = mergedChunk.slice(mergedChunk.size - rest);
        feedBackParseRef.chunks.push(restChunk);
        feedBackParseRef.totalSize += restChunk.size;
      }
      const wordsChunk = mergedChunk.slice(0, mergedChunk.size - rest);
      const resultWords: number[] = [];
      try {
        const ab = await wordsChunk.arrayBuffer();
        const dv = new DataView(ab);

        for (let i = 0; i < ab.byteLength; i = i + wordSize) {
          // ab is data from network stream (so it's big endian)
          const word = dv.getUint32(i, false);
          resultWords.push(word);
        }
        return resultWords;
      } catch (e) {
        console.error("failed to get arraybuffer from blob", e);
      }
    }
  };
  return new TransformStream({
    async transform(chunk, controller) {
      feedBackParseRef.chunks.push(chunk);
      if (chunk instanceof ArrayBuffer) {
        const chunkSize = (chunk as ArrayBuffer).byteLength;
        feedBackParseRef.totalSize += chunkSize;
      } else if (chunk instanceof Blob) {
        const chunkSize = (chunk as Blob).size;
        feedBackParseRef.totalSize += chunkSize;
      } else {
        console.error(
          "[dbg] [uint32streamparser] chunk has unknown binary type",
          chunk,
        );
      }
      const resultWords = await doConsume();
      for (const word of resultWords ?? []) {
        controller.enqueue(word);
      }
    },
    async flush(controller) {
      const resultWords = await doConsume();
      for (const word of resultWords ?? []) {
        controller.enqueue(word);
      }
    },
  });
}
