import { RefObject } from "react";
import {
  ChatMessage,
  ChatMessagePing,
  ChatMessagePingDirection,
  EchoDirectionC2S,
  EchoDirectionS2C,
  EchoPayload,
  MessagePayload,
  PingStateRef,
} from "./types";

export interface PingRTTSample {
  rttMs: number;
  seq: number;
  timeout?: boolean;
}

export function createPingStreamFromDC(
  pingDC: RTCDataChannel,
  fromNodeId: string,
  toNodeId: string,
  pingTimeoutMs: number,
) {
  const pingSeqRef: PingStateRef = { seq: 0, txMap: {}, timer: undefined };
  return new ReadableStream<PingRTTSample>({
    start(controller) {
      pingDC.onerror = (ev) => {
        console.error(`[dbg] ping data channel error`, ev, "dc", pingDC);
        controller.error(ev.error);
      };
      pingDC.onclose = () => {
        if (pingSeqRef.timer) {
          clearInterval(pingSeqRef.timer);
          pingSeqRef.timer = undefined;
        }
        controller.close();
      };
      pingDC.onmessage = (ev) => {
        try {
          const msgObject: ChatMessage = JSON.parse(ev.data);
          if (
            msgObject.ping &&
            msgObject.ping.direction === ChatMessagePingDirection.Pong
          ) {
            const seq = msgObject.ping.seq;
            if (seq !== undefined && seq !== null) {
              const txTime = pingSeqRef?.txMap[seq];
              if (txTime !== undefined && txTime !== null) {
                const rtt = Date.now() - txTime;
                delete pingSeqRef?.txMap[seq];
                controller.enqueue({ rttMs: rtt, seq });
              } else {
                controller.enqueue({ rttMs: NaN, seq, timeout: true });
              }
            }
          }
        } catch (e) {
          console.error("failed to parse ping data channel message", e);
        }
      };
      pingDC.onopen = () => {
        pingSeqRef.timer = setInterval(() => {
          const pingPayload: ChatMessagePing = {
            direction: ChatMessagePingDirection.Ping,
            seq: pingSeqRef.seq,
          };
          pingSeqRef.seq++;
          const pingMsg: ChatMessage = {
            messageId: crypto.randomUUID(),
            timestamp: Date.now(),
            fromNodeId: fromNodeId,
            toNodeId: toNodeId,
            ping: pingPayload,
          };
          pingSeqRef.txMap[pingPayload.seq] = Date.now();
          pingDC.send(JSON.stringify(pingMsg));
          const seq = pingPayload.seq;
          setTimeout(() => {
            if (seq in pingSeqRef.txMap) {
              delete pingSeqRef.txMap[seq];
            }
          }, pingTimeoutMs);
        }, 1000);
      };
    },
    cancel(reason: any) {
      console.debug(
        `[dbg] ping stream is cancelled, dc:`,
        pingDC,
        "reason",
        reason,
      );
    },
  });
}

export function setupWsPing(
  pingIntvMs: number,
  ws: WebSocket,
  pingTxMapRef: RefObject<Record<string, number>>,
  pingCorrId: string,
) {
  const seqRef: { current: number } = { current: 0 };
  return setInterval(() => {
    const seq = seqRef.current ?? 0;
    const echoMsg: MessagePayload = {
      echo: {
        direction: EchoDirectionC2S,
        correlation_id: pingCorrId,
        server_timestamp: 0,
        timestamp: Date.now(),
        seq_id: seq,
      },
    };
    const nextSeq = seq + 1;
    seqRef.current = nextSeq;
    pingTxMapRef.current[seq.toString()] = Date.now();
    ws.send(JSON.stringify(echoMsg));
  }, pingIntvMs);
}

export function handleWsEcho(
  echo: EchoPayload,
  correlationId: string,
  pingTxMapRef: RefObject<Record<string, number>>,
  onSample: (rttMs: number, seq: number, timeout: boolean) => void,
) {
  if (
    echo.direction === EchoDirectionS2C &&
    echo.correlation_id === correlationId
  ) {
    const now = Date.now();
    const t0 = pingTxMapRef.current[echo.seq_id.toString()];
    if (t0 !== undefined) {
      const rtt = now - t0;
      onSample(rtt, echo.seq_id, false);
    }
  }
}
