"use client";

import {
  ChatMessage,
  ChatMessageFile,
  ChatMessagePing,
  ChatMessagePingDirection,
  ConnEntry,
  ConnTrackEntry,
  ConnTrackStatus,
  ConnTrackStatusEntry,
  EchoDirectionC2S,
  EchoDirectionS2C,
  FileTransferStatusEntry,
  ICEOfferPayload,
  MessagePayload,
  OfferType,
  PingStateRef,
  PredefinedDCLabel,
  RegisterPayload,
  RenamePayload,
  SDPOfferPayload,
} from "@/apis/types";
import { ChangeNameDialog } from "@/components/InputDialog";
import { Box, Button } from "@mui/material";
import {
  Dispatch,
  Fragment,
  RefObject,
  SetStateAction,
  useMemo,
  useRef,
  useState,
} from "react";
import { LeftPanel } from "@/components/LeftPanel";
import { getConns } from "@/apis/conns";
import { BasicWsInfo } from "@/components/BasicWsInfo";
import { RenderPeerEntry } from "@/components/RenderPeerEntry";
import { RenderMessage } from "@/components/RenderMessage";
import { MessageComposer } from "@/components/MessageComposer";
import {
  createStreamFromDataChannel,
  newUint32StreamParser,
  wordSize,
} from "@/utls/streams";

const googleStunServer = "stun:stun.l.google.com:19302";
const pingTimeoutMs = 3000;
const wsAddr = "ws://localhost:3001/ws";
const pingIntvMs = 1000;
const defaultFileSegmentSize = 128 * 1024;

function makeConnTrackEntry(): ConnTrackEntry {
  return {
    peerConnection: new RTCPeerConnection({
      iceServers: [{ urls: googleStunServer }],
    }),
    remoteOffers: [],
    queuedICEOffers: [],
  };
}

// key is the node_id of remote peer
type ConnTrack = Record<string, ConnTrackEntry>;

function useWs(setConnTrackStatus: Dispatch<SetStateAction<ConnTrackStatus>>) {
  const [connected, setConnected] = useState(false);
  const [connecting, setConnecting] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const correlationId = useMemo(() => crypto.randomUUID(), []);

  const [nodeId, setNodeId] = useState<string>("");
  const nodeIdRef = useRef<string>("");
  const seqRef = useRef(0);
  const pingTxMapRef = useRef<Record<string, number>>({});
  const [rtt, setRtt] = useState<number | undefined>(undefined);
  const [lastSeq, setLastSeq] = useState<number | undefined>(undefined);
  const connectedAtRef = useRef<number | undefined>(undefined);
  const [upTime, setUpTime] = useState<number | undefined>(undefined);
  const pingTimerRef = useRef<NodeJS.Timeout | undefined>(undefined);
  const [conns, setConns] = useState<ConnEntry[]>([]);
  const connTrackRef = useRef<ConnTrack>({});

  const doRefresh = () =>
    getConns().then((conns) => {
      setConns(conns);
    });

  const doConnect = (addr: string) => {
    setConnecting(true);
    const ws = new WebSocket(addr);
    wsRef.current = ws;

    ws.onopen = () => {
      connectedAtRef.current = Date.now();
      setConnected(true);
      setConnecting(false);
      const registerPayload: RegisterPayload = {
        node_name: "",
      };
      const registerMsg: MessagePayload = {
        register: registerPayload,
      };
      ws.send(JSON.stringify(registerMsg));

      pingTimerRef.current = setInterval(() => {
        const seq = seqRef.current ?? 0;
        const echoMsg: MessagePayload = {
          echo: {
            direction: EchoDirectionC2S,
            correlation_id: correlationId,
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
    };
    const cleanUp = () => {
      setConnected(false);
      setConnecting(false);
      setConns([]);
      if (pingTimerRef.current) {
        clearInterval(pingTimerRef.current);
        pingTimerRef.current = undefined;
      }
      connTrackRef.current = {};
      nodeIdRef.current = "";
      setNodeId("");
    };
    ws.onclose = () => {
      cleanUp();
    };
    ws.onerror = (error) => {
      console.error("[dbg] ws error", error);
      cleanUp();
    };
    ws.onmessage = (event) => {
      const logSource = "acceptor";
      try {
        const msg: MessagePayload = JSON.parse(event.data);
        const echo = msg.echo;
        if (echo && echo.direction === EchoDirectionS2C) {
          const now = Date.now();
          const t0 = pingTxMapRef.current[echo.seq_id.toString()];
          if (t0 !== undefined) {
            const rtt = now - t0;
            setRtt(rtt);
            setLastSeq(echo.seq_id);

            const connectedAt = connectedAtRef.current;
            if (connectedAt !== undefined) {
              const upTime = now - (connectedAt ?? 0);
              setUpTime(upTime);
            }

            if (echo.seq_id === 0) {
              getConns().then((conns) => {
                setConns(conns);
              });
            }
          }
        }
        if (msg.online) {
          doRefresh();
        }
        if (msg.register) {
          if (msg.node_id) {
            setNodeId(msg.node_id);
            nodeIdRef.current = msg.node_id;
          }
          doRefresh();
        }
        if (msg.rename) {
          doRefresh();
        }
        if (msg.sdp_offer && msg.sdp_offer.to_node_id === nodeIdRef.current) {
          console.log(
            `[dbg]${logSource} got SDP offer of type`,
            msg.sdp_offer.type,
            "from remote peer",
            msg.sdp_offer,
          );
          const remoteNodeId = msg.sdp_offer.from_node_id;
          if (!(remoteNodeId in connTrackRef.current)) {
            // automatically accept the SDP offer
            console.log(
              `[dbg] [${logSource}] automatically accepting SDP offer from remote peer`,
              remoteNodeId,
            );
            const ent = makeConnTrackEntry();
            connTrackRef.current[remoteNodeId] = ent;
            console.log(
              `[dbg] [${logSource}] initializing conn track entry for remote peer`,
              remoteNodeId,
              ent,
            );

            attachPeerConnectionEventListeners(
              ent.peerConnection,
              setConnTrackStatus,
              remoteNodeId,
              nodeIdRef,
              wsRef,
              logSource,
            );
          }
          const ent = connTrackRef.current[remoteNodeId];
          if (
            ent.peerConnection.ondatachannel === null ||
            ent.peerConnection.ondatachannel === undefined
          ) {
            ent.peerConnection.ondatachannel = (event) => {
              console.log(`[dbg] [${logSource}] on data channel`, event);
              const dc = event.channel;
              attachDCEventListeners(
                dc,
                setConnTrackStatus,
                remoteNodeId,
                logSource,
              );
              if (dc.label === PredefinedDCLabel.Chat) {
                ent.dataChannel = dc;
              } else if (dc.label === PredefinedDCLabel.File) {
                const dcKey = dc.id?.toString();
                if (!dcKey) {
                  console.error(
                    `[dbg] [${logSource}] data channel id is not a string`,
                    dc.id,
                  );
                  return;
                }
                if (!ent.fileDataChannels) {
                  ent.fileDataChannels = {};
                }
                if (!(dcKey in ent.fileDataChannels)) {
                  ent.fileDataChannels[dcKey] = dc;
                  // todo: listen events from File DC
                }
              } else if (dc.label === PredefinedDCLabel.Ping) {
                // no op
              } else {
                console.error(
                  `[dbg] [${logSource}] unknown data channel label`,
                  dc.label,
                );
              }
            };
          }

          const entry = connTrackRef.current[remoteNodeId];
          try {
            const offer = JSON.parse(msg.sdp_offer.offer_json);
            entry.remoteOffers.push(offer);
            entry.peerConnection.setRemoteDescription(offer);
            if (msg.sdp_offer.type === OfferType.Offer) {
              console.log(
                `[dbg] [${logSource}] creating answer for SDP offer from remote peer`,
                remoteNodeId,
              );
              entry.peerConnection
                .createAnswer()
                .then((answerOffer) => {
                  console.log(
                    "[dbg] offer of type answer successfully created for remote peer",
                    remoteNodeId,
                    answerOffer,
                  );
                  entry.peerConnection.setLocalDescription(answerOffer);
                  const answerPayload: SDPOfferPayload = {
                    type: OfferType.Answer,
                    offer_json: JSON.stringify(answerOffer),
                    from_node_id: nodeIdRef.current,
                    to_node_id: remoteNodeId,
                  };
                  const answerMsg: MessagePayload = {
                    sdp_offer: answerPayload,
                  };
                  const wsConn = wsRef.current;
                  if (wsConn) {
                    wsConn.send(JSON.stringify(answerMsg));
                  } else {
                    console.error(
                      "failed to reply SDP offer to remote peer",
                      remoteNodeId,
                      "because ws connection is not established",
                    );
                  }
                })
                .catch((e) =>
                  console.log(
                    "failed to create answer for SDP offer from remote peer",
                    e,
                  ),
                );
            }
            if (entry.queuedICEOffers.length > 0) {
              console.log(
                `[dbg] [${logSource}] adding queued ICE offers to peer connection`,
                remoteNodeId,
              );
              for (const queuedICEOffer of entry.queuedICEOffers) {
                entry.peerConnection.addIceCandidate(queuedICEOffer);
              }
              entry.queuedICEOffers = [];
            }
          } catch (e) {
            console.log("failed to handle remote SDP offer", e);
          }
        }
        if (msg.ice_offer && msg.ice_offer.to_node_id === nodeIdRef.current) {
          console.log("[dbg] got ICE offer from remote peer", msg.ice_offer);
          const remoteNodeId = msg.ice_offer.from_node_id;
          if (!(remoteNodeId in connTrackRef.current)) {
            // implies that the ICE offer is arrived too early, or we didn't prepared for that.
            console.error(
              "got ICE offer from remote peer",
              remoteNodeId,
              "but no conn track entry found",
            );
            return;
          }
          if (connTrackRef.current[remoteNodeId].remoteOffers.length === 0) {
            // queue the ICE offer
            try {
              const offer = JSON.parse(msg.ice_offer.offer_json);
              connTrackRef.current[remoteNodeId].queuedICEOffers.push(offer);
            } catch (e) {
              console.log("failed to parse remote ICE offer", e);
            }
            console.log(
              "[dbg] ice offer from remote peer",
              remoteNodeId,
              "is queued",
            );
            return;
          }
          console.log(
            "[dbg] adding ICE offer to peer connection",
            remoteNodeId,
          );
          try {
            const offer = JSON.parse(msg.ice_offer.offer_json);
            connTrackRef.current[remoteNodeId].peerConnection.addIceCandidate(
              offer,
            );
          } catch (e) {
            console.log(
              "failed to add ICE offer to peer connection",
              remoteNodeId,
              e,
            );
          }
        }
      } catch (e) {
        console.error("Failed to handle ws message", e);
      }
    };
  };

  return {
    rtt,
    lastSeq,
    upTime,
    nodeId,
    nodeIdRef,
    conns,
    connected,
    connecting,
    wsRef,
    doConnect,
    connTrackRef,
  };
}

function updateConnTrackStatusByMsgObject(
  prev: ConnTrackStatus,
  remoteNodeId: string,
  msgObject: ChatMessage,
) {
  let amendedMsgObject: ChatMessage | undefined;
  let amendedMsgId: string | undefined;
  if (msgObject.amend) {
    amendedMsgObject = JSON.parse(msgObject.amend.newMessageJSON);
    amendedMsgId = msgObject.amend.messageId;
  }

  const theEntry = prev[remoteNodeId] ? { ...prev[remoteNodeId] } : {};
  let messages = theEntry.messages ? [...theEntry.messages] : [];

  if (amendedMsgObject && amendedMsgId) {
    const idx = messages.findIndex(
      (msgObj) => msgObj.messageId === amendedMsgId,
    );
    if (idx !== -1) {
      messages[idx] = amendedMsgObject;
    }
  } else if (msgObject.delete && msgObject.delete.messageId) {
    messages = messages.filter(
      (msgObj) => msgObj.messageId !== msgObject.delete!.messageId,
    );
  } else {
    const idx = messages.findIndex(
      (msg) => msg.messageId === msgObject.messageId,
    );
    if (idx === -1) {
      messages.push(msgObject);
    }
  }

  theEntry.messages = messages;
  return {
    ...prev,
    [remoteNodeId]: theEntry,
  };
}

function updateFileTransferStatusEntryByDCData(
  prev: FileTransferStatusEntry,
  dc: RTCDataChannel,
  data: Blob | ArrayBuffer | number,
): FileTransferStatusEntry {
  const chunkSize =
    typeof data === "number"
      ? data
      : data instanceof Blob
        ? data.size
        : data instanceof ArrayBuffer
          ? data.byteLength
          : 0;

  return {
    ...prev,
    bytesReceived: (prev.bytesReceived ?? 0) + chunkSize,
    chunksReceived: (prev.chunksReceived ?? 0) + 1,
    chunksMetadata: [
      ...(prev.chunksMetadata ?? []),
      { seq: prev.chunksReceived ?? 0, blobType: dc.binaryType },
    ],
    blobChunks:
      dc.binaryType === "blob" && data instanceof Blob
        ? [...(prev?.blobChunks ?? []), data as Blob]
        : undefined,
    arrayBufferChunks:
      dc.binaryType === "arraybuffer" && data instanceof ArrayBuffer
        ? [...(prev?.arrayBufferChunks ?? []), data as ArrayBuffer]
        : undefined,
  };
}

function updateConnTrackStatusEntryByDCData(
  prev: ConnTrackStatusEntry,
  dc: RTCDataChannel,
  data: Blob | ArrayBuffer | number,
) {
  const dcId = dc.id?.toString()!;
  if (!dcId) {
    return prev;
  }
  return {
    ...prev,
    fileTransferStatus: {
      ...(prev?.fileTransferStatus ?? {}),
      [dcId]: updateFileTransferStatusEntryByDCData(
        prev?.fileTransferStatus?.[dcId] ?? { bytesReceived: 0 },
        dc,
        data,
      ),
    },
  };
}

// both the sender and the receiver can call this function to update the status of file transfer
function updateConnTrackStatusByDCData(
  prev: ConnTrackStatus,
  remoteNodeId: string,
  dc: RTCDataChannel,
  data: Blob | ArrayBuffer | number,
) {
  const dcId = dc.id?.toString();
  if (!dcId) {
    return prev;
  }

  const connTrackStatus = {
    ...prev,
    [remoteNodeId]: updateConnTrackStatusEntryByDCData(
      prev[remoteNodeId] ?? {},
      dc,
      data,
    ),
  };

  return connTrackStatus;
}

function sendAckToDC(
  dc: RTCDataChannel,
  fromNodeId: string,
  toNodeId: string,
  messageId: string,
) {
  const msgObject: ChatMessage = {
    fromNodeId,
    toNodeId,
    timestamp: Date.now(),
    messageId: crypto.randomUUID(),
    ack: {
      messageId,
    },
  };
  dc.send(JSON.stringify(msgObject));
}

function closeDCById(
  prev: ConnTrackStatus,
  remoteNodeId: string,
  dcId: string,
  error: Error | undefined,
  originFile: File | undefined,
) {
  const url = originFile
    ? URL.createObjectURL(originFile)
    : URL.createObjectURL(
        new Blob(
          prev[remoteNodeId]?.fileTransferStatus?.[dcId]?.arrayBufferChunks ??
            [],
        ),
      );

  return {
    ...prev,
    [remoteNodeId]: {
      ...(prev[remoteNodeId] ?? {}),
      messages: prev[remoteNodeId]?.messages?.map((msg) =>
        msg.file && msg.file?.dcId === dcId
          ? {
              ...msg,
              file: {
                ...msg.file,
                dcId: undefined,
                url: url,
                originFile: originFile,
                chunks:
                  prev[remoteNodeId]?.fileTransferStatus?.[dcId]
                    ?.arrayBufferChunks ?? [],
              },
            }
          : msg,
      ),
      fileTransferStatus: {
        ...(prev[remoteNodeId]?.fileTransferStatus ?? {}),
        [dcId]: undefined,
      },
    },
  } as ConnTrackStatus;
}

function sendFeedBackToDC(
  dc: RTCDataChannel,
  chunkSizeAny: number | ArrayBuffer | Blob,
) {
  const chunkSize =
    typeof chunkSizeAny === "number"
      ? chunkSizeAny
      : chunkSizeAny instanceof Blob
        ? chunkSizeAny.size
        : chunkSizeAny instanceof ArrayBuffer
          ? chunkSizeAny.byteLength
          : 0;

  const ab = new ArrayBuffer(wordSize);
  new DataView(ab).setUint32(0, chunkSize, false);
  dc.send(ab);
}

function createFileTransferStatusEntry(
  prev: ConnTrackStatus,
  remoteNodeId: string,
  dcId: string,
): ConnTrackStatus {
  return {
    ...prev,
    [remoteNodeId]: {
      ...(prev[remoteNodeId] ?? {}),
      fileTransferStatus: {
        [dcId]: { bytesReceived: 0 },
      },
    },
  };
}

function attachDCEventListeners(
  dc: RTCDataChannel,
  setConnTrackStatus: Dispatch<SetStateAction<ConnTrackStatus>>,
  remoteNodeId: string,
  logId: string,
) {
  const logSource = logId ? ` [${logId}]` : "";
  dc.onopen = () => {
    console.log(`[dbg]${logSource} data channel opened`, dc, "dcId", dc.id);
    if (dc.label === PredefinedDCLabel.File) {
      dc.binaryType = "arraybuffer";
      // for zero-byte file transfer, the onmessage event of the DC might not necessarily fires,
      // so we need to do this to handle zero-byte file transfer (i.e. to transfer some file that has zero bytes of data)
      // and this will not overrride the real data, so the order of arrival of onopen event and onmessage event doesn't matter.
      const dcId = dc.id?.toString();
      if (dcId) {
        setConnTrackStatus((prev) => {
          return createFileTransferStatusEntry(prev, remoteNodeId, dcId);
        });
      }

      sendFeedBackToDC(dc, 0);
    }
  };

  dc.onclose = () => {
    console.log(`[dbg]${logSource} data channel closed`, dc);
    if (dc.label === PredefinedDCLabel.File) {
      const dcId = dc.id?.toString();
      if (dcId) {
        setConnTrackStatus((prev) => {
          return closeDCById(prev, remoteNodeId, dcId, undefined, undefined);
        });
      }
    }
  };

  dc.onerror = (error) => {
    console.error(`[dbg]${logSource} data channel error`, error);
    if (dc.label === PredefinedDCLabel.File) {
      const dcId = dc.id?.toString();
      if (dcId) {
        setConnTrackStatus((prev) => {
          return closeDCById(prev, remoteNodeId, dcId, error.error, undefined);
        });
      }
    }
  };

  if (dc.label === PredefinedDCLabel.Chat) {
    dc.onmessage = (event) => {
      try {
        const msgObject: ChatMessage = JSON.parse(event.data);

        if (msgObject.ack) {
          // the receiver just sent us an ack, so we update the list of messages
          // and we SHOULD NOT send back an ack (this would cause a loop)
          const msgId = msgObject.ack.messageId;
          setConnTrackStatus((prev) => {
            return {
              ...prev,
              [remoteNodeId]: {
                ...(prev[remoteNodeId] ?? {}),
                messages:
                  prev[remoteNodeId]?.messages?.map((msg) =>
                    msg.messageId === msgId ? { ...msg, acked: true } : msg,
                  ) ?? [],
              },
            };
          });
        } else {
          setConnTrackStatus((prev) => {
            return updateConnTrackStatusByMsgObject(
              prev,
              remoteNodeId,
              msgObject,
            );
          });
          // send back the ack to the sender
          sendAckToDC(
            dc,
            msgObject.toNodeId,
            msgObject.fromNodeId,
            msgObject.messageId,
          );
        }
      } catch (e) {
        console.error("failed to parse data channel chat message", e);
      }
    };
  } else if (dc.label === PredefinedDCLabel.File) {
    dc.onmessage = (event) => {
      setConnTrackStatus((prev) => {
        return updateConnTrackStatusByDCData(
          prev,
          remoteNodeId,
          dc,
          event.data,
        );
      });
      sendFeedBackToDC(dc, event.data);
    };
  } else if (dc.label === PredefinedDCLabel.Ping) {
    dc.onmessage = (event) => {
      try {
        const msgObject: ChatMessage = JSON.parse(event.data);
        if (msgObject.ping) {
          const replyPayload: ChatMessagePing = {
            direction: ChatMessagePingDirection.Pong,
            seq: msgObject.ping.seq,
          };
          const replyMsg: ChatMessage = {
            messageId: crypto.randomUUID(),
            timestamp: Date.now(),
            fromNodeId: msgObject.toNodeId ?? "",
            toNodeId: msgObject.fromNodeId ?? "",
            ping: replyPayload,
          };
          dc.send(JSON.stringify(replyMsg));
        }
      } catch (e) {
        console.error("failed to parse data channel ping message", e);
      }
    };
  }
}

function attachPeerConnectionEventListeners(
  peerConnection: RTCPeerConnection,
  setConnTrackStatus: Dispatch<SetStateAction<ConnTrackStatus>>,
  remoteNodeId: string,
  nodeIdRef: RefObject<string>,
  wsRef: RefObject<WebSocket | null>,
  logId?: string,
) {
  const logSource = logId ? ` [${logId}]` : "";
  // registering event handlers for peerconnection handle
  peerConnection.oniceconnectionstatechange = (event) => {
    console.log(`[dbg]${logSource} ice connection state changed`, event);
    setConnTrackStatus((prev) => ({
      ...prev,
      [remoteNodeId]: {
        ...prev[remoteNodeId],
        connecting: false,
      },
    }));

    if (
      peerConnection.iceConnectionState === "connected" ||
      peerConnection.iceConnectionState === "completed"
    ) {
      setConnTrackStatus((prev) => ({
        ...prev,
        [remoteNodeId]: {
          ...prev[remoteNodeId],
          disconnected: false,
        },
      }));
    }
    if (peerConnection.iceConnectionState === "checking") {
      setConnTrackStatus((prev) => ({
        ...prev,
        [remoteNodeId]: {
          ...prev[remoteNodeId],
          connecting: true,
        },
      }));
    }
    if (peerConnection.iceConnectionState === "disconnected") {
      setConnTrackStatus((prev) => ({
        ...prev,
        [remoteNodeId]: {
          ...prev[remoteNodeId],
          disconnected: true,
        },
      }));
    }

    if (peerConnection.iceConnectionState === "failed") {
      // see: https://developer.mozilla.org/en-US/docs/Web/API/WebRTC_API/Session_lifetime#ice_restart
      // todo:  pc.setConfiguration(restartConfig);
      setConnTrackStatus((prev) => ({
        ...prev,
        [remoteNodeId]: {
          ...prev[remoteNodeId],
          disconnected: true,
        },
      }));
      console.log(
        `[dbg]${logSource} ice connection to peer`,
        remoteNodeId,
        "state changed to failed, doing ICE-restart after 1 second",
      );
      setTimeout(() => {
        console.log(`[dbg]${logSource} restarting ICE for peer`, remoteNodeId);
        peerConnection.restartIce();
        setConnTrackStatus((prev) => ({
          ...prev,
          [remoteNodeId]: {
            ...prev[remoteNodeId],
            connecting: true,
          },
        }));

        console.log(
          `[dbg]${logSource} creating iceRestart offer for peer`,
          remoteNodeId,
        );
        peerConnection
          .createOffer({ iceRestart: true })
          .then((offer) => {
            console.log(
              `[dbg]${logSource} iceRestart offer created for peer`,
              remoteNodeId,
              offer,
            );
            const offerPayload: SDPOfferPayload = {
              type: OfferType.Offer,
              offer_json: JSON.stringify(offer),
              from_node_id: nodeIdRef.current,
              to_node_id: remoteNodeId,
            };
            const offerMsg: MessagePayload = {
              sdp_offer: offerPayload,
            };
            wsRef.current?.send(JSON.stringify(offerMsg));
          })
          .catch((e) => {
            console.error(
              `[dbg]${logSource} failed to create iceRestart offer for peer`,
              remoteNodeId,
              e,
            );
          });
      }, 1000);
    }
  };

  peerConnection.onicecandidate = (event) => {
    const candidate = event.candidate;
    if (candidate && candidate !== null) {
      // We found a candidate! Send it to the peer immediately.
      console.log(`[dbg]${logSource} new local ICE candidate:`, candidate);

      const iceOfferPayload: ICEOfferPayload = {
        offer_json: JSON.stringify(candidate),
        from_node_id: nodeIdRef.current,
        to_node_id: remoteNodeId,
      };
      const iceOfferMsg: MessagePayload = {
        ice_offer: iceOfferPayload,
      };
      wsRef.current?.send(JSON.stringify(iceOfferMsg));
    } else {
      // If event.candidate is null, it means the browser
      // has finished gathering all possible candidates.
      console.log(`[dbg]${logSource} End of ICE candidate gathering.`);
    }
  };
}

export default function Home() {
  const [connTrackStatus, setConnTrackStatus] = useState<ConnTrackStatus>({});

  const {
    rtt,
    lastSeq,
    upTime,
    nodeId,
    nodeIdRef,
    conns,
    connected,
    connecting,
    wsRef,
    doConnect,
    connTrackRef,
  } = useWs(setConnTrackStatus);

  const name = conns
    ? conns.find((conn) => conn.node_id === nodeId)?.entry?.node_name
    : undefined;

  const [nameEdited, setNameEdited] = useState<string>("");
  const [activeConn, setActiveConn] = useState("");
  const [showChangeName, setShowChangeName] = useState(false);
  const switchActiveConn = (remoteNodeId: string) => {
    const logSource = "initiator";
    setActiveConn(remoteNodeId);

    let ent = connTrackRef.current[remoteNodeId];
    if (!ent) {
      ent = makeConnTrackEntry();
      connTrackRef.current[remoteNodeId] = ent;

      attachPeerConnectionEventListeners(
        ent.peerConnection,
        setConnTrackStatus,
        remoteNodeId,
        nodeIdRef,
        wsRef,
        logSource,
      );

      ent.dataChannel = ent.peerConnection.createDataChannel(
        PredefinedDCLabel.Chat,
      );

      attachDCEventListeners(
        ent.dataChannel,
        setConnTrackStatus,
        remoteNodeId,
        logSource,
      );

      ent.peerConnection
        .createOffer()
        .then((offer) => {
          const offerPayload: SDPOfferPayload = {
            type: OfferType.Offer,
            offer_json: JSON.stringify(offer),
            from_node_id: nodeIdRef.current,
            to_node_id: remoteNodeId,
          };
          const offerMsg: MessagePayload = {
            sdp_offer: offerPayload,
          };
          ent.peerConnection.setLocalDescription(offer);
          wsRef.current?.send(JSON.stringify(offerMsg));
        })
        .catch((e) => {
          console.error(
            `[dbg] [${logSource}] failed to create offer to remote peer`,
            remoteNodeId,
            e,
          );
        });
    }

    if (ent && (ent.pingSeqRef === undefined || ent.pingSeqRef === null)) {
      const pingDC = ent.peerConnection.createDataChannel(
        PredefinedDCLabel.Ping,
      );
      pingDC.onerror = (ev) => {
        console.error(`[dbg] [${logSource}] ping data channel error`, ev);
      };
      pingDC.onclose = (ev) => {
        if (ent.pingSeqRef) {
          if (
            ent.pingSeqRef.timer !== undefined &&
            ent.pingSeqRef.timer !== null
          ) {
            clearInterval(ent.pingSeqRef.timer);
            ent.pingSeqRef.timer = undefined;
          }
          ent.pingSeqRef = undefined;
          setConnTrackStatus((prev) => ({
            ...prev,
            [remoteNodeId]: {
              ...prev[remoteNodeId],
              rtt: undefined,
            },
          }));
        }
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
              const txTime = ent.pingSeqRef?.txMap[seq];
              if (txTime !== undefined && txTime !== null) {
                const rtt = Date.now() - txTime;
                delete ent.pingSeqRef?.txMap[seq];
                setConnTrackStatus((prev) => ({
                  ...prev,
                  [remoteNodeId]: {
                    ...prev[remoteNodeId],
                    rtt: rtt,
                  },
                }));
              } else {
                // timeout, the trace entry has already been deleted
                // todo: maybe display it properly in the UI ?
              }
            }
          }
        } catch (e) {
          console.error("failed to parse ping data channel message", e);
        }
      };
      const pingSeqRef: PingStateRef = { seq: 0, txMap: {} };
      ent.pingSeqRef = pingSeqRef;
      pingDC.onopen = (ev) => {
        pingSeqRef.timer = setInterval(() => {
          const pingPayload: ChatMessagePing = {
            direction: ChatMessagePingDirection.Ping,
            seq: pingSeqRef.seq,
          };
          pingSeqRef.seq++;
          const pingMsg: ChatMessage = {
            messageId: crypto.randomUUID(),
            timestamp: Date.now(),
            fromNodeId: nodeIdRef.current,
            toNodeId: remoteNodeId,
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
    }
  };

  // for messages sent by ourselves, it has to be acked before it can appear on the screen.
  const messages: ChatMessage[] =
    connTrackStatus[activeConn]?.messages?.filter(
      (msg) => msg.acked || msg.fromNodeId !== nodeId,
    ) ?? [];

  const sendMsg = (msgObject: ChatMessage, toNodeId: string) => {
    connTrackRef.current[toNodeId]?.dataChannel?.send(
      JSON.stringify(msgObject),
    );
    setConnTrackStatus((prev) => {
      return updateConnTrackStatusByMsgObject(prev, toNodeId, msgObject);
    });
  };

  const sendAmendMsg = (amendMsgObject: ChatMessage) => {
    const msgToSend: ChatMessage = {
      timestamp: Date.now(),
      fromNodeId: nodeIdRef.current,
      toNodeId: amendMsgObject.toNodeId,
      messageId: crypto.randomUUID(),
      amend: {
        messageId: amendMsgObject.messageId,
        newMessageJSON: JSON.stringify(amendMsgObject),
      },
    };
    const dc = connTrackRef.current[amendMsgObject.toNodeId]?.dataChannel;
    if (dc) {
      dc.send(JSON.stringify(msgToSend));
      setConnTrackStatus((prev) => {
        return updateConnTrackStatusByMsgObject(
          prev,
          amendMsgObject.toNodeId,
          msgToSend,
        );
      });
    }
  };

  const sendMsgDeleteRequest = (toNodeId: string, deletingMsgId: string) => {
    const msgToSend: ChatMessage = {
      timestamp: Date.now(),
      fromNodeId: nodeIdRef.current,
      toNodeId: toNodeId,
      messageId: crypto.randomUUID(),
      delete: {
        messageId: deletingMsgId,
      },
    };
    const dc = connTrackRef.current[toNodeId]?.dataChannel;
    if (dc) {
      dc.send(JSON.stringify(msgToSend));
      setConnTrackStatus((prev) => {
        return updateConnTrackStatusByMsgObject(prev, toNodeId, msgToSend);
      });
    }
  };

  let usernameMap: Record<string, string> = {};
  for (const conn of conns ?? []) {
    if (conn.entry?.node_name) {
      usernameMap = { ...usernameMap, [conn.node_id]: conn.entry.node_name };
    }
  }

  // for test purpose
  if (!(nodeId in usernameMap)) {
    usernameMap = { ...usernameMap, [nodeId]: "Me" };
  }

  return (
    <Fragment>
      <Box sx={{ display: "flex", flexDirection: "row", height: "100vh" }}>
        <LeftPanel>
          <Box>
            <Box>
              {connected ? (
                <Box>
                  <BasicWsInfo
                    name={name}
                    url={wsRef?.current?.url}
                    rtt={rtt}
                    nodeId={nodeId}
                    lastSeq={lastSeq}
                    upTime={upTime}
                    onNameChangeRequested={() => {
                      setNameEdited(name ?? "");
                      setShowChangeName(true);
                    }}
                  />
                  <Box>
                    <Box sx={{ padding: 2 }}>Peers</Box>
                    <Box>
                      {conns
                        .filter((conn) => conn.node_id !== nodeId)
                        .map((conn) => (
                          <RenderPeerEntry
                            conn={conn}
                            key={conn.node_id}
                            activeNodeId={activeConn}
                            onSelect={() => switchActiveConn(conn.node_id)}
                            rtt={connTrackStatus?.[conn.node_id]?.rtt}
                          />
                        ))}
                    </Box>
                  </Box>
                </Box>
              ) : (
                <Box>
                  <Button
                    loading={connecting}
                    onClick={() => {
                      doConnect(wsAddr);
                    }}
                  >
                    Connect
                  </Button>
                </Box>
              )}
            </Box>
          </Box>
        </LeftPanel>
        <Box
          sx={{
            flex: 1,
            minWidth: 0,
            display: "flex",
            flexDirection: "column",
            overflow: "hidden",
          }}
        >
          <Box
            sx={{
              flex: 1,
              minHeight: 0,
              overflow: "auto",
              display: "flex",
              flexDirection: "column",
              gap: 1,
              padding: 2,
            }}
          >
            {messages.map((message) => (
              <RenderMessage
                message={message}
                key={message.messageId}
                onAmend={(amendedMsg) => {
                  sendAmendMsg(amendedMsg);
                }}
                onDelete={(deletedMsgId) => {
                  sendMsgDeleteRequest(activeConn, deletedMsgId);
                }}
                fileTransferStatus={
                  connTrackStatus?.[activeConn]?.fileTransferStatus ?? {}
                }
                usernameMap={usernameMap}
              />
            ))}
          </Box>
          <Box sx={{ flexShrink: 0 }}>
            <MessageComposer
              onFile={(filelist) => {
                if (filelist && filelist.length > 0) {
                  for (const file of filelist) {
                    const pc = connTrackRef.current[activeConn]?.peerConnection;
                    if (pc) {
                      const fileDC = pc.createDataChannel(
                        PredefinedDCLabel.File,
                      );
                      fileDC.binaryType = "arraybuffer";
                      fileDC.onopen = () => {
                        const dcId = fileDC.id?.toString() || "";
                        const msgObject: ChatMessage = {
                          messageId: crypto.randomUUID(),
                          timestamp: Date.now(),
                          fromNodeId: nodeIdRef.current,
                          toNodeId: activeConn,
                          file: {
                            name: file.name,
                            type: file.type,
                            size: file.size,
                            dcId: dcId,
                          },
                        };
                        sendMsg(msgObject, msgObject.toNodeId);
                        setConnTrackStatus((prev) => {
                          return createFileTransferStatusEntry(
                            prev,
                            msgObject.toNodeId,
                            dcId,
                          );
                        });

                        const fbStream = createStreamFromDataChannel(
                          fileDC,
                        ).pipeThrough(newUint32StreamParser());
                        const fbReader = fbStream.getReader();
                        let fbRef: { receivedTotalBytes: number } = {
                          receivedTotalBytes: 0,
                        };
                        const doReadFeedBackStream = ({
                          value,
                          done,
                        }: {
                          value: any;
                          done: boolean;
                        }) => {
                          if (done) {
                            return;
                          }
                          const chunkSize = value as number;

                          setConnTrackStatus((prev) => {
                            return updateConnTrackStatusByDCData(
                              prev,
                              msgObject.toNodeId,
                              fileDC,
                              chunkSize,
                            );
                          });

                          fbRef.receivedTotalBytes += chunkSize;
                          if (fbRef.receivedTotalBytes >= file.size) {
                            // all chunks have been confirmed to be received by the receiver of the file transfer
                            fileDC.close();
                          }
                          fbReader.read().then(doReadFeedBackStream);
                        };
                        fbReader
                          .read()
                          .then(doReadFeedBackStream)
                          .catch((e) =>
                            console.error("failed to read feed back stream", e),
                          );

                        let sentSizeRef: { value: number } = { value: 0 };

                        const doSendChunk = (maxSize: number) => {
                          const offset = sentSizeRef.value;
                          const endLimit = Math.min(
                            sentSizeRef.value + defaultFileSegmentSize,
                            file.size,
                            offset + maxSize + 1,
                          );
                          if (endLimit > offset) {
                            const chunk = file.slice(offset, endLimit);
                            if (chunk.size > 0) {
                              sentSizeRef.value += chunk.size;
                              const s = chunk.size;
                              try {
                                fileDC.send(chunk);
                                return s;
                              } catch (e) {
                                console.error("failed to send chunk", e);
                              }
                            }
                          }
                          return 0;
                        };

                        fileDC.bufferedAmountLowThreshold = 4 * 1024 * 1024;
                        const doSendChunks = () => {
                          let freeSpace =
                            fileDC.bufferedAmountLowThreshold -
                            fileDC.bufferedAmount;
                          while (freeSpace >= 0) {
                            const s = doSendChunk(freeSpace);
                            if (s === 0) {
                              break;
                            }
                            freeSpace -= s;
                          }
                        };
                        doSendChunks();
                        fileDC.onbufferedamountlow = (event) => {
                          doSendChunks();
                        };

                        fileDC.onclose = () => {
                          setConnTrackStatus((prev) => {
                            return closeDCById(
                              prev,
                              activeConn,
                              dcId,
                              undefined,
                              file,
                            );
                          });
                        };
                        fileDC.onerror = (ev) => {
                          setConnTrackStatus((prev) => {
                            return closeDCById(
                              prev,
                              activeConn,
                              dcId,
                              ev.error,
                              file,
                            );
                          });
                        };
                      };
                    }
                  }
                }
              }}
              onPhoto={(filelist) => {
                // todo
                // if (filelist && filelist.length > 0) {
                //   for (const file of filelist) {
                //     const msgObject: ChatMessage = {
                //       messageId: crypto.randomUUID(),
                //       timestamp: Date.now(),
                //       fromNodeId: nodeIdRef.current,
                //       toNodeId: activeConn,
                //     };
                //     const filePayload: ChatMessageFile = {
                //       url: "",
                //       name: file.name,
                //       type: file.type,
                //       size: file.size,
                //     };
                //     if (file.type.startsWith("image/")) {
                //       msgObject.image = filePayload;
                //     } else if (file.type.startsWith("video/")) {
                //       msgObject.video = filePayload;
                //     } else {
                //       msgObject.file = filePayload;
                //     }
                //     sendMsg(msgObject, activeConn);
                //   }
                // }
              }}
              onText={(text) => {
                const msgObject: ChatMessage = {
                  messageId: crypto.randomUUID(),
                  message: text,
                  timestamp: Date.now(),
                  fromNodeId: nodeIdRef.current,
                  toNodeId: activeConn,
                };
                sendMsg(msgObject, activeConn);
              }}
            />
          </Box>
        </Box>
      </Box>
      <ChangeNameDialog
        name={nameEdited}
        onNameChange={(name) => {
          setNameEdited(name);
        }}
        open={showChangeName}
        onClose={() => {
          setShowChangeName(false);
        }}
        onConfirm={(newName) => {
          return new Promise((resolve) => {
            const renamePayload: RenamePayload = {
              new_node_name: newName,
              origin_node_name: name,
            };
            const renameMsg: MessagePayload = {
              rename: renamePayload,
            };
            wsRef.current?.send(JSON.stringify(renameMsg));

            resolve();
            setShowChangeName(false);
          });
        }}
      />
    </Fragment>
  );
}
