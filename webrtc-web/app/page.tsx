"use client";

import {
  ChatMessage,
  ChatMessageFileCategory,
  ChatMessageThumbnail,
  ChatMessagePing,
  ChatMessagePingDirection,
  ConnEntry,
  ConnTrackEntry,
  ConnTrackStatus,
  ConnTrackStatusEntry,
  FileTransferStatusEntry,
  ICEOfferPayload,
  MessagePayload,
  OfferType,
  PredefinedDCLabel,
  RegisterPayload,
  RenamePayload,
  SDPOfferPayload,
  WellKnownAttributes,
  Preference,
  WSServer,
  MessagePatchesMap,
  WSConnStatusShort,
} from "@/apis/types";
import { useAutoconnect } from "@/apis/autoconnect";
import { ChangePreference } from "@/components/ChangePreference";
import {
  Box,
  Tooltip,
  IconButton,
  MenuItem,
  TextField,
  Paper,
  Drawer,
  useMediaQuery,
  useTheme,
  Menu,
} from "@mui/material";
import {
  Dispatch,
  Fragment,
  RefObject,
  SetStateAction,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { LeftPanel } from "@/components/LeftPanel";
import { getConns } from "@/apis/conns";
import { RenderPeerEntry } from "@/components/RenderPeerEntry";
import { RenderMessage } from "@/components/RenderMessage";
import { MessageComposer } from "@/components/MessageComposer";
import {
  createStreamFromDataChannel,
  newUint32StreamParser,
  wordSize,
} from "@/utls/streams";
import { Edit, Menu as MenuIcon } from "@mui/icons-material";
import { RenderAvatar } from "@/components/RenderAvatar";
import { createThumbnailFromFile } from "@/apis/thumbnail";
import { useUnreads } from "@/apis/unreads";
import { useScrollTop } from "@/apis/scrollTop";
import { unmarshalMessagePatchOrder } from "@/apis/message_patch";
import { appendWsPathToCurrentOrigin, getSignallingServers } from "@/apis/ws";
import { ServerSelector } from "@/components/ServerSelector";
import { useFileDrop } from "@/components/useFileDrop";
import {
  createPingStreamFromDC,
  handleWsEcho,
  PingRTTSample,
  setupWsPing,
} from "@/apis/ping";
import { PSKey, usePersistentStorage } from "@/apis/persistent";
import { ConnStatusDisplay } from "@/components/ConnStatusDisplay";
import { useLoginStatusPolling } from "@/apis/profile";
import { useQuery } from "@tanstack/react-query";
import { logout } from "@/apis/logout";
import { usePreference } from "@/apis/preference";

const pingTimeoutMs = 3000;
const pingIntvMs = 1000;
const defaultFileSegmentSize = 128 * 1024;
const defaultMsgTimeoutMs = 3000;
const defaultFileDCBufferAmountThreshold = 4 * 1024 * 1024;
const pollingIntvMs = 3000;
const reconnectDelayMs = 3000;

function makeConnTrackEntry(iceServers: string[]): ConnTrackEntry {
  console.debug("[dbg] making RTCPeerConnection, iceServers:", iceServers);
  return {
    peerConnection: new RTCPeerConnection({
      iceServers: [{ urls: iceServers }],
    }),
    remoteOffers: [],
    queuedICEOffers: [],
  };
}

// key is the node_id of remote peer
type ConnTrack = Record<string, ConnTrackEntry>;

interface WSConnRecord {
  serverId: string;
  ws: WebSocket;
  shouldReconnect: boolean;
}

function useWs(
  setConnTrackStatus: Dispatch<SetStateAction<ConnTrackStatus>>,
  setMsgPatches: Dispatch<SetStateAction<MessagePatchesMap>>,
  audioCtxRef: RefObject<AudioContext | null>,
) {
  const wsRef = useRef<WebSocket | null>(null);
  const correlationId = useMemo(() => crypto.randomUUID(), []);
  const { addUnreadMessageIds } = useUnreads();

  const [nodeId, setNodeId] = useState<string>("");
  const nodeIdRef = useRef<string>("");
  const pingTxMapRef = useRef<Record<string, number>>({});
  const [rtt, setRtt] = useState<number | undefined>(undefined);
  const [lastSeq, setLastSeq] = useState<number | undefined>(undefined);
  const connectedAtRef = useRef<number | undefined>(undefined);
  const [upTime, setUpTime] = useState<number | undefined>(undefined);
  const pingTimerRef = useRef<NodeJS.Timeout | undefined>(undefined);
  const [conns, setConns] = useState<ConnEntry[]>([]);
  const connTrackRef = useRef<ConnTrack>({});
  const [wsConnStatus, setWSConnStatus] = useState<WSConnStatusShort>(
    WSConnStatusShort.Unknown,
  );

  const doRefresh = (apiPrefix: string) =>
    getConns(apiPrefix).then((conns) => {
      setConns(conns);
    });

  const allWsConnsRef = useRef<WSConnRecord[]>([]);

  const sendWsMsg = (obj: any) => {
    try {
      let j = "";
      if (typeof obj === "string") {
        j = obj;
      } else {
        j = JSON.stringify(obj);
      }
      if (!j) {
        return;
      }
      wsRef.current?.send(j);
      if (!wsRef.current) {
        console.error("object msg", obj, "didn't send");
      }
    } catch (e) {
      console.error("Failed to marshal object to JSON:", e, obj);
    }
  };

  const connect = (server: WSServer, preference: Preference | undefined) => {
    if (allWsConnsRef.current.find((rec) => rec.serverId === server.id)) {
      console.log(
        `Server ${server.id} already has connection record, skipping for singleton.`,
      );
      return;
    }
    console.log("Connecting to ", server);

    const addr = appendWsPathToCurrentOrigin(server.url);
    const iceServers = server.iceServers;

    setWSConnStatus(WSConnStatusShort.Connecting);
    const ws = new WebSocket(addr);
    wsRef.current = ws;

    const cleanUp = () => {
      setConns([]);
      if (pingTimerRef.current !== null && pingTimerRef.current !== undefined) {
        clearInterval(pingTimerRef.current);
        pingTimerRef.current = undefined;
      }
      connTrackRef.current = {};
      nodeIdRef.current = "";
      setNodeId("");
    };

    const logSource = "acceptor";

    ws.onopen = () => {
      setWSConnStatus(WSConnStatusShort.Online);
      connectedAtRef.current = Date.now();
      const registerPayload: RegisterPayload = {
        node_name: preference?.name ?? "",
      };
      const registerMsg: MessagePayload = {
        register: registerPayload,
        attributes_announcement: {
          attributes: {
            [WellKnownAttributes.SupportAttachment]: "true",
            [WellKnownAttributes.PreferredColor]:
              preference?.indexOfPreferColor !== undefined &&
              preference.indexOfPreferColor !== null
                ? String(preference.indexOfPreferColor)
                : undefined,
          },
        },
      };
      sendWsMsg(registerMsg);

      pingTimerRef.current = setupWsPing(
        pingIntvMs,
        ws,
        pingTxMapRef,
        correlationId,
      );
    };

    ws.onclose = () => {
      setWSConnStatus(WSConnStatusShort.Offline);
      cleanUp();

      const shouldReconnect = !!wsRef.current;
      if (shouldReconnect) {
        console.log("Disconnected, will reconnect later");
        setTimeout(() => {
          connect(server, preference);
        }, reconnectDelayMs);
      }
    };
    ws.onerror = (error) => {
      console.error(`[dbg] [${logSource}] ws error`, error);
    };
    ws.onmessage = (event) => {
      try {
        const msg: MessagePayload = JSON.parse(event.data);
        if (msg.echo) {
          handleWsEcho(msg.echo, correlationId, pingTxMapRef, (rttMs, seq) => {
            setRtt(rttMs);
            setLastSeq(seq);
            const connectedAt = connectedAtRef.current;
            if (connectedAt !== undefined && connectedAt !== null) {
              setUpTime(new Date().valueOf() - connectedAt);
            }
            if (seq == 0) {
              doRefresh(server.apiPrefix);
            }
          });
        }

        if (msg.online) {
          doRefresh(server.apiPrefix);
        }
        if (msg.register) {
          if (msg.node_id) {
            setNodeId(msg.node_id);
            nodeIdRef.current = msg.node_id;
          }
          doRefresh(server.apiPrefix);
        }
        if (msg.rename) {
          doRefresh(server.apiPrefix);
        }
        if (msg.sdp_offer && msg.sdp_offer.to_node_id === nodeIdRef.current) {
          console.log(
            `[dbg] [${logSource}] got SDP offer of type`,
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
            const ent = makeConnTrackEntry(iceServers);
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
              sendWsMsg,
              logSource,
              audioCtxRef,
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
                setMsgPatches,
                remoteNodeId,
                logSource,
                (unreads) =>
                  addUnreadMessageIds(nodeIdRef.current || "", unreads),
              );
              if (dc.label === PredefinedDCLabel.Chat) {
                ent.dataChannel = dc;
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
                    `[dbg] [${logSource}] offer of type answer successfully created for remote peer`,
                    remoteNodeId,
                    answerOffer,
                  );
                  return entry.peerConnection.setLocalDescription(answerOffer);
                })
                .then(() => {
                  const answerPayload: SDPOfferPayload = {
                    type: OfferType.Answer,
                    offer_json: JSON.stringify(
                      entry.peerConnection.localDescription,
                    ),
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
                      `[${logSource}] failed to reply SDP offer to remote peer`,
                      remoteNodeId,
                      "because ws connection is not established",
                    );
                  }
                })
                .catch((e) =>
                  console.error(
                    `[${logSource}] failed to create answer for SDP offer from remote peer`,
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
                entry.peerConnection
                  .addIceCandidate(queuedICEOffer)
                  .catch((e) => {
                    console.error(
                      `[${logSource}] failed to add queued ICE candidate to peer connection`,
                      remoteNodeId,
                      e,
                    );
                  });
              }
              entry.queuedICEOffers = [];
            }
          } catch (e) {
            console.error(
              `[${logSource}] failed to handle remote SDP offer`,
              e,
            );
          }
        }
        if (msg.ice_offer && msg.ice_offer.to_node_id === nodeIdRef.current) {
          console.log(
            `[dbg] [${logSource}] got ICE offer from remote peer`,
            msg.ice_offer,
          );
          const remoteNodeId = msg.ice_offer.from_node_id;
          if (!(remoteNodeId in connTrackRef.current)) {
            // implies that the ICE offer is arrived too early, or we didn't prepared for that.
            console.error(
              `[${logSource}] got ICE offer from remote peer`,
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
              console.error(
                `[${logSource}] failed to parse remote ICE offer`,
                e,
              );
            }

            console.log(
              `[dbg] [${logSource}] ice offer from remote peer`,
              remoteNodeId,
              "is queued",
              "queue:",
              connTrackRef.current?.[remoteNodeId]?.queuedICEOffers,
            );
            return;
          }
          console.log(
            `[dbg] [${logSource}] adding ICE offer to peer connection`,
            remoteNodeId,
          );
          try {
            const offer = JSON.parse(msg.ice_offer.offer_json);
            connTrackRef.current[remoteNodeId].peerConnection
              .addIceCandidate(offer)
              .catch((e) => {
                console.error(
                  `[${logSource}] failed to add ICE candidate to peer connection`,
                  remoteNodeId,
                  e,
                );
              });
          } catch (e) {
            console.log(
              `[${logSource}] failed to parse remote ICE offer JSON`,
              e,
            );
          }
        }
      } catch (e) {
        console.error(`[${logSource}] Failed to handle ws message`, e);
      }
    };
  };

  const disconnect = () => {
    if (wsRef.current) {
      const ws = wsRef.current;
      // set the current value of the ref to null to disable auto-reconnect
      wsRef.current = null;
      try {
        ws.close?.();
      } catch (e) {
        console.error("failed to close ws:", e);
      }
    }
  };

  return {
    rtt,
    lastSeq,
    upTime,
    nodeId,
    nodeIdRef,
    conns,
    connTrackRef,
    wsConnStatus,
    sendWsMsg,
    connect,
    disconnect,
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
  binaryType: BinaryType,
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
      { seq: prev.chunksReceived ?? 0, blobType: binaryType },
    ],
    arrayBufferChunks:
      binaryType === "arraybuffer" && data instanceof ArrayBuffer
        ? [...(prev?.arrayBufferChunks ?? []), data as ArrayBuffer]
        : undefined,
  };
}

function updateConnTrackStatusEntryByDCData(
  prev: ConnTrackStatusEntry,
  dcId: string,
  binaryType: BinaryType,
  data: Blob | ArrayBuffer | number,
) {
  return {
    ...prev,
    fileTransferStatus: {
      ...(prev?.fileTransferStatus ?? {}),
      [dcId]: updateFileTransferStatusEntryByDCData(
        prev?.fileTransferStatus?.[dcId] ?? { bytesReceived: 0 },
        binaryType,
        data,
      ),
    },
  };
}

// both the sender and the receiver can call this function to update the status of file transfer
function updateConnTrackStatusByDCData(
  prev: ConnTrackStatus,
  remoteNodeId: string,
  dcID: string,
  binaryType: BinaryType,
  data: Blob | ArrayBuffer | number,
) {
  const connTrackStatus = {
    ...prev,
    [remoteNodeId]: updateConnTrackStatusEntryByDCData(
      prev[remoteNodeId] ?? {},
      dcID,
      binaryType,
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
  const activeMsg = prev[remoteNodeId]?.messages?.find(
    (msg) => msg.file?.dcId === dcId,
  );
  const url = originFile
    ? URL.createObjectURL(originFile)
    : URL.createObjectURL(
        new File(
          prev[remoteNodeId]?.fileTransferStatus?.[dcId]?.arrayBufferChunks ??
            [],
          activeMsg?.file?.name ?? "unknownfile",
          { type: activeMsg?.file?.type ?? "application/octet-stream" },
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
                error: error,
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
        ...(prev[remoteNodeId]?.fileTransferStatus ?? {}),
        [dcId]: { bytesReceived: 0 },
      },
    },
  };
}

function attachDCEventListeners(
  dc: RTCDataChannel,
  setConnTrackStatus: Dispatch<SetStateAction<ConnTrackStatus>>,
  setMsgPatches: Dispatch<SetStateAction<MessagePatchesMap>>,
  remoteNodeId: string,
  logId: string,
  onUnread: (msgIds: string[]) => void,
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
    } else if (dc.label === PredefinedDCLabel.Chat) {
      setConnTrackStatus((prev) => ({
        ...prev,
        [remoteNodeId]: {
          ...(prev[remoteNodeId] ?? {}),
          readyToTalk: true,
        },
      }));
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
    } else if (dc.label === PredefinedDCLabel.Chat) {
      setConnTrackStatus((prev) => ({
        ...prev,
        [remoteNodeId]: {
          ...(prev[remoteNodeId] ?? {}),
          readyToTalk: false,
        },
      }));
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
          onUnread([msgObject.messageId]);
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
          dc.id?.toString() ?? "",
          dc.binaryType,
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
  } else if (dc.label == PredefinedDCLabel.MsgStream) {
    dc.onmessage = (event) => {
      console.log(
        `[dbg]${logSource} message from dc, label: ${dc.label}, data:`,
        event.data,
      );
      if (!(event.data instanceof ArrayBuffer)) {
        console.error(
          `[dbg]${logSource} message from dc, label: ${dc.label}, data:`,
          event.data,
          "is not of type ArrayBuffer",
        );
        return;
      }
      const data = event.data as ArrayBuffer;
      try {
        const patchOrder = unmarshalMessagePatchOrder(data);
        console.log(
          `[dbg]${logSource} parsed message patch order:`,
          patchOrder,
        );
        if (patchOrder) {
          setMsgPatches((prev) => ({
            ...prev,
            [patchOrder.MessageID]: [
              ...(prev[patchOrder.MessageID] || []),
              patchOrder,
            ],
          }));
        }
      } catch (err) {
        console.error(
          `[dbg]${logSource} message from dc, label: ${dc.label}, data:`,
          event.data,
          "can not be parsed:",
          err,
        );
      }
    };
  }
}

function attachPeerConnectionEventListeners(
  peerConnection: RTCPeerConnection,
  setConnTrackStatus: Dispatch<SetStateAction<ConnTrackStatus>>,
  remoteNodeId: string,
  nodeIdRef: RefObject<string>,
  sendWsMsg: (obj: any) => void,
  logId: string | undefined,
  audioCtxRef: RefObject<AudioContext | null>,
) {
  const logSource = logId ? ` [${logId}]` : "";

  if (!audioCtxRef.current) {
    audioCtxRef.current = new AudioContext();
    console.log(
      `[dbg]${logSource} audio context created:`,
      audioCtxRef.current,
    );
  }

  peerConnection.onconnectionstatechange = () => {
    setConnTrackStatus((prev) => ({
      ...prev,
      [remoteNodeId]: {
        ...(prev[remoteNodeId] ?? {}),
        connectionStatus: peerConnection.connectionState,
      },
    }));
  };

  // registering event handlers for peerconnection handle
  peerConnection.oniceconnectionstatechange = (event) => {
    console.log(
      `[dbg]${logSource} iceconnection state changed`,
      event,
      "change to:",
      peerConnection.iceConnectionState,
    );
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
    if (
      peerConnection.iceConnectionState === "checking" ||
      (peerConnection.iceConnectionState as any) === "connecting"
    ) {
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
            return peerConnection.setLocalDescription(offer);
          })
          .then(() => {
            const offerPayload: SDPOfferPayload = {
              type: OfferType.Offer,
              offer_json: JSON.stringify(peerConnection.localDescription),
              from_node_id: nodeIdRef.current,
              to_node_id: remoteNodeId,
            };
            const offerMsg: MessagePayload = {
              sdp_offer: offerPayload,
            };
            sendWsMsg(offerMsg);
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
      sendWsMsg(iceOfferMsg);
    } else {
      // If event.candidate is null, it means the browser
      // has finished gathering all possible candidates.
      console.log(`[dbg]${logSource} End of ICE candidate gathering.`);
    }
  };

  peerConnection.ontrack = (event) => {
    console.log(
      `[dbg]${logSource} Received track open request from peer ${remoteNodeId}:`,
      event.track,
    );

    if (event.track.kind !== "audio") {
      // because the 'ontrack' event fires once for every track (e.g. once for audio, then once for video)
      console.log(
        `[dbg]${logSource} Skipping non-audio track from peer ${remoteNodeId}:`,
        event.track,
      );
      return;
    }

    event.track.onended = (ev) => {
      console.log(
        `[dbg]${logSource} Audio track ended from peer ${remoteNodeId}:`,
        event.track,
        ev,
      );
    };

    setConnTrackStatus((prev) => {
      return {
        ...prev,
        [remoteNodeId]: {
          ...(prev[remoteNodeId] ?? {}),
          messages: [
            ...(prev[remoteNodeId]?.messages ?? []),
            {
              messageId: crypto.randomUUID(),
              fromNodeId: remoteNodeId,
              toNodeId: nodeIdRef.current,
              timestamp: Date.now(),
              songTrack: {
                label: event.track.label,
                track: event.track,
              },
            },
          ],
        },
      };
    });
  };
}

function tryParseInt(s: string): number {
  try {
    const n = parseInt(s);
    if (isNaN(n)) {
      return -1;
    }
    return n;
  } catch (e) {
    console.error("failed to parse int:", s, e);
  }
  return -1;
}

function getUserPreferenceMap(conns: ConnEntry[]): Record<string, Preference> {
  const userPreferenceMap: Record<string, Preference> = {};
  for (const conn of conns ?? []) {
    if (conn.entry?.node_name) {
      userPreferenceMap[conn.node_id] = {
        name: conn.entry.node_name,
        indexOfPreferColor: tryParseInt(
          conn.entry?.attributes?.[WellKnownAttributes.PreferredColor] ?? "",
        ),
      };
    }
  }

  return userPreferenceMap;
}

// onAck will always get called, regardless of whether there is an error or timeout.
function listenForAck(
  dc: RTCDataChannel,
  msgId: string,
  timeoutMs: number,
  onAck: (timeout: boolean, error?: Error) => void,
) {
  const timeoutRef: { timeoutId: NodeJS.Timeout | undefined } = {
    timeoutId: undefined,
  };

  function handleEv(ev: MessageEvent) {
    try {
      const evData = JSON.parse(ev.data) as ChatMessage;
      if (evData.ack && evData.ack.messageId === msgId) {
        onAck(false, undefined);
        if (timeoutRef.timeoutId) {
          clearTimeout(timeoutRef.timeoutId);
          timeoutRef.timeoutId = undefined;
        }
        dc.removeEventListener("message", handleEv);
        return;
      }
    } catch (error) {
      onAck(false, error as Error);
    }
  }

  dc.addEventListener("message", handleEv);
  timeoutRef.timeoutId = setTimeout(() => {
    onAck(true, undefined);
    dc.removeEventListener("message", handleEv);
  }, timeoutMs);
}

// the caller has to ensure that the fileDC is opened before calling this function.
function transmitFileData(
  fileDC: RTCDataChannel,
  file: File,
  onProgress: (
    totalBytesTransferred: number,
    chunkSizeJustTransferred: number,
  ) => void,
) {
  console.log("[dbg] [transmit] file", file.name);
  const fbStream = createStreamFromDataChannel(fileDC).pipeThrough(
    newUint32StreamParser(),
  );

  const fbReader = fbStream.getReader();

  const fbRef: { receivedTotalBytes: number } = {
    receivedTotalBytes: 0,
  };

  const doReadFeedBackStream = ({
    value,
    done,
  }: {
    value: unknown;
    done: boolean;
  }) => {
    if (done) {
      return;
    }
    const chunkSize = value as number;

    fbRef.receivedTotalBytes += chunkSize;
    onProgress(fbRef.receivedTotalBytes, chunkSize);
    if (fbRef.receivedTotalBytes >= file.size) {
      // all chunks have been confirmed to be received by the receiver of the file transfer
      fileDC.close();
    }
    fbReader.read().then(doReadFeedBackStream);
  };

  fbReader
    .read()
    .then(doReadFeedBackStream)
    .catch((e) => console.error("failed to read feed back stream", e));

  const sentSizeRef: { value: number } = {
    value: 0,
  };

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

  fileDC.bufferedAmountLowThreshold = defaultFileDCBufferAmountThreshold;

  const doSendChunks = () => {
    let freeSpace = fileDC.bufferedAmountLowThreshold - fileDC.bufferedAmount;
    while (freeSpace >= 0) {
      const s = doSendChunk(freeSpace);
      if (s === 0) {
        break;
      }
      freeSpace -= s;
    }
  };

  doSendChunks();
  fileDC.onbufferedamountlow = () => {
    doSendChunks();
  };
}

function sendMsg(
  dc: RTCDataChannel,
  msgObject: ChatMessage,
  toNodeId: string,
  setConnTrackStatus: Dispatch<SetStateAction<ConnTrackStatus>>,
): Promise<ChatMessage> {
  setConnTrackStatus((prev) => {
    return updateConnTrackStatusByMsgObject(prev, toNodeId, msgObject);
  });

  return new Promise((resolve, reject) => {
    listenForAck(
      dc,
      msgObject.messageId,
      defaultMsgTimeoutMs,
      (timeout, err) => {
        if (timeout) {
          reject(
            new Error(`timeout: message ${msgObject.messageId} timed out`),
          );
          return;
        }
        if (err) {
          reject(
            new Error(
              `error: message ${msgObject.messageId} failed to be acked: ${err.message}`,
            ),
          );
          return;
        }
        resolve(msgObject);
      },
    );
    dc.send(JSON.stringify(msgObject));
  });
}

function transmitFileViaPC(
  pc: RTCPeerConnection,
  chatDC: RTCDataChannel,
  fromNodeId: string,
  toNodeId: string,
  fileCat: ChatMessageFileCategory,
  file: File,
  setConnTrackStatus: Dispatch<SetStateAction<ConnTrackStatus>>,
  thumbnail: ChatMessageThumbnail | undefined,
) {
  const fileDC = pc.createDataChannel(PredefinedDCLabel.File);
  fileDC.binaryType = "arraybuffer";
  fileDC.onopen = () => {
    const dcId = fileDC.id?.toString() || "";
    console.log("[dbg] [dcId] dcId", dcId);
    const msgObject: ChatMessage = {
      messageId: crypto.randomUUID(),
      timestamp: Date.now(),
      fromNodeId: fromNodeId,
      toNodeId: toNodeId,
      file: {
        category: fileCat,
        name: file.name,
        type: file.type,
        size: file.size,
        dcId: dcId,
        thumbnail: thumbnail,
      },
    };

    sendMsg(chatDC, msgObject, msgObject.toNodeId, setConnTrackStatus)
      .then((msgObject) => {
        setConnTrackStatus((prev) => {
          return createFileTransferStatusEntry(prev, msgObject.toNodeId, dcId);
        });

        console.log("[dbg] [acked] file", file.name);

        transmitFileData(fileDC, file, (_, chunk) => {
          setConnTrackStatus((prev) => {
            return updateConnTrackStatusByDCData(
              prev,
              msgObject.toNodeId,
              dcId,
              fileDC.binaryType,
              chunk,
            );
          });
        });

        fileDC.onclose = () => {
          setConnTrackStatus((prev) => {
            return closeDCById(prev, toNodeId, dcId, undefined, file);
          });
        };
        fileDC.onerror = (ev) => {
          setConnTrackStatus((prev) => {
            return closeDCById(prev, toNodeId, dcId, ev.error, file);
          });
        };
      })
      .catch((e) => {
        console.error("failed to send(or ack) message", e);
      });
  };
}

function getVisibleMessageIds(
  msgsBoxRef: RefObject<HTMLDivElement | null>,
): string[] {
  const visibleIds: string[] = [];
  const msgsBox = msgsBoxRef.current;
  if (!msgsBox) {
    return visibleIds;
  }

  const scrollTop = msgsBox.scrollTop;
  const containerViewPortHeight = msgsBox.clientHeight;
  const messageElements = msgsBox.querySelectorAll("[data-message-id]");

  messageElements.forEach((el) => {
    const htmlEl = el as HTMLElement;
    const messageId = htmlEl.getAttribute("data-message-id");
    const relativeOffset = htmlEl.offsetTop;
    const isVisible =
      relativeOffset >= scrollTop &&
      relativeOffset < scrollTop + containerViewPortHeight;

    // console.log(
    //   `[dbg] message ${messageId}: offset=${relativeOffset.toFixed(2)}px, scrollTop=${scrollTop.toFixed(2)}px, containerHeight=${containerViewPortHeight.toFixed(2)}px, visible=${isVisible}`,
    // );

    if (isVisible && messageId) {
      visibleIds.push(messageId);
    }
  });

  return visibleIds;
}

function sortConnsByLatestUnread(
  connEntries: ConnEntry[],
  connTrackStatus: ConnTrackStatus,
  unreadMsgIds: Set<string>,
) {
  return connEntries.sort((connA, connB) => {
    const messagesA = connTrackStatus[connA.node_id]?.messages ?? [];
    const messagesB = connTrackStatus[connB.node_id]?.messages ?? [];

    // Get unread messages for each connection (messages from the remote peer that are unread)
    const unreadA = messagesA.filter((msg) => unreadMsgIds.has(msg.messageId));
    const unreadB = messagesB.filter((msg) => unreadMsgIds.has(msg.messageId));

    // If connA has unread messages and connB doesn't, connA comes first
    if (unreadA.length > 0 && unreadB.length === 0) {
      return -1;
    }

    // If connB has unread messages and connA doesn't, connB comes first
    if (unreadB.length > 0 && unreadA.length === 0) {
      return 1;
    }

    // If both have unread messages, compare the latest unread message timestamp
    if (unreadA.length > 0 && unreadB.length > 0) {
      const latestA = Math.max(...unreadA.map((msg) => msg.timestamp));
      const latestB = Math.max(...unreadB.map((msg) => msg.timestamp));
      return latestB - latestA; // Newer messages first
    }

    // Neither has unread messages, compare the last message timestamp
    if (messagesA.length > 0 && messagesB.length > 0) {
      const lastMsgA = messagesA[messagesA.length - 1].timestamp;
      const lastMsgB = messagesB[messagesB.length - 1].timestamp;
      return lastMsgB - lastMsgA; // Newer messages first
    }

    // If only one has messages, that one comes first
    if (messagesA.length > 0) {
      return -1;
    }
    if (messagesB.length > 0) {
      return 1;
    }

    // Neither has messages, maintain original order
    return 0;
  });
}

function getPeerUnreadMsgs(
  connTrackStatus: ConnTrackStatus,
  peerId: string,
  unreadSet: Set<string>,
): ChatMessage[] {
  const peerMessages = connTrackStatus?.[peerId]?.messages ?? [];

  const unreadPeerMsgs = peerMessages.filter((msg) =>
    unreadSet.has(msg.messageId),
  );
  return unreadPeerMsgs;
}

function createAndSendOffer(
  pc: RTCPeerConnection,
  sendWsMsg: (obj: any) => void,
  localNodeId: string,
  remoteNodeId: string,
  logSource: string = "",
) {
  return pc
    .createOffer()
    .then((offer) => pc.setLocalDescription(offer))
    .then(() => {
      const offerPayload: SDPOfferPayload = {
        type: OfferType.Offer,
        offer_json: JSON.stringify(pc.localDescription),
        from_node_id: localNodeId,
        to_node_id: remoteNodeId,
      };
      const offerMsg: MessagePayload = {
        sdp_offer: offerPayload,
      };
      sendWsMsg(offerMsg);
      console.log(`[dbg]${logSource} SDP offer sent to peer`, remoteNodeId);
    })
    .catch((err) => {
      console.error(
        `[dbg]${logSource} Failed to create and send SDP offer:`,
        err,
      );
      throw err; // Re-throw so caller can handle
    });
}

function determineFollowingMode(msgsBox: HTMLDivElement) {
  const { scrollHeight, clientHeight, scrollTop } = msgsBox;
  const isScrollable = scrollHeight > clientHeight;
  const isNearBottom = scrollTop + clientHeight >= scrollHeight - 50; // 50px threshold

  // Following mode is on when:
  // 1. Container is scrollable and scrolled near the bottom
  // 2. Container is not scrollable (not enough messages)
  return !isScrollable || isNearBottom;
}

export default function Home() {
  const [connTrackStatus, setConnTrackStatus] = useState<ConnTrackStatus>({});
  const [msgPatches, setMsgPatches] = useState<MessagePatchesMap>({});
  const audioCtxRef = useRef<AudioContext | null>(null);

  const { data: servers = [] } = useQuery<WSServer[]>({
    queryKey: ["signallingServers"],
    queryFn: () => getSignallingServers(),
  });

  const pinnedSrvSt = usePersistentStorage(PSKey.PinnedServer);
  const pinnedServer = pinnedSrvSt.getValue() || "";
  const setPinnedServer = (s: string) => pinnedSrvSt.setValue(s);
  const pinnedserverObject = servers?.find(
    (server) => server.id === pinnedServer,
  );
  const selectedserverObject = pinnedserverObject;
  const { loggedIn, loggedInAs, clearLoggedInState } = useLoginStatusPolling(
    selectedserverObject?.apiPrefix || "",
    pollingIntvMs,
  );

  const {
    rtt,
    lastSeq,
    upTime,
    nodeId,
    nodeIdRef,
    conns,
    connTrackRef,
    wsConnStatus,
    sendWsMsg,
    connect,
    disconnect,
  } = useWs(setConnTrackStatus, setMsgPatches, audioCtxRef);

  const name = conns
    ? conns.find((conn) => conn.node_id === nodeId)?.entry?.node_name
    : undefined;

  const { preference, setPreference } = usePreference();

  // auto-connect at sever list loaded
  useAutoconnect(pinnedserverObject, preference, connect);

  const [activeConn, setActiveConn] = useState("");
  const [showPreferenceDialog, setShowPreferenceDialog] = useState(false);
  const [mobileDrawerOpen, setMobileDrawerOpen] = useState(false);

  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down("md"));

  const switchActiveConn = (
    remoteNodeId: string,
    iceServers: string[],
    onUnread: (msgIds: string[]) => void,
  ) => {
    const logSource = "initiator";
    setActiveConn(remoteNodeId);

    let ent = connTrackRef.current[remoteNodeId];
    if (!ent) {
      ent = makeConnTrackEntry(iceServers);
      connTrackRef.current[remoteNodeId] = ent;

      attachPeerConnectionEventListeners(
        ent.peerConnection,
        setConnTrackStatus,
        remoteNodeId,
        nodeIdRef,
        sendWsMsg,
        logSource,
        audioCtxRef,
      );

      ent.dataChannel = ent.peerConnection.createDataChannel(
        PredefinedDCLabel.Chat,
      );

      ent.pingDC = ent.peerConnection.createDataChannel(PredefinedDCLabel.Ping);
      const pingSampleStream = createPingStreamFromDC(
        ent.pingDC,
        nodeIdRef.current,
        remoteNodeId,
        pingTimeoutMs,
      );

      const reader = pingSampleStream.getReader();
      const doRead = (
        streamDataEv: ReadableStreamReadResult<PingRTTSample>,
      ) => {
        if (streamDataEv.done) {
          return;
        }
        const value = streamDataEv.value;
        if (value) {
          setConnTrackStatus((prev) => ({
            ...prev,
            [remoteNodeId]: {
              ...prev[remoteNodeId],
              rtt: value.rttMs,
            },
          }));
        }
        reader.read().then(doRead);
      };
      reader.read().then(doRead);

      attachDCEventListeners(
        ent.dataChannel,
        setConnTrackStatus,
        setMsgPatches,
        remoteNodeId,
        logSource,
        onUnread,
      );

      createAndSendOffer(
        ent.peerConnection,
        sendWsMsg,
        nodeIdRef.current,
        remoteNodeId,
      ).catch((e) => {
        console.error(
          `[dbg] [${logSource}] failed to create offer to remote peer`,
          remoteNodeId,
          e,
        );
      });
    }
  };

  // for messages sent by ourselves, it has to be acked before it can appear on the screen.
  const messages: ChatMessage[] =
    connTrackStatus[activeConn]?.messages?.filter(
      (msg) => msg.acked || msg.fromNodeId !== nodeId,
    ) ?? [];

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
      timestamp: new Date().valueOf(),
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

  const userPreferenceMap = getUserPreferenceMap(conns ?? []);

  const [searchKw, setSearchKw] = useState<string>("");

  const msgsBoxRef = useRef<HTMLDivElement>(null);

  const followingModeRef = useRef(false);

  // scroll to last message when following mode is on
  useEffect(() => {
    const mutObs = new MutationObserver((mutations) => {
      if (followingModeRef.current) {
        for (const mutation of mutations) {
          if (mutation.type === "childList") {
            if (mutation.addedNodes.length > 0) {
              (mutation.addedNodes[0] as HTMLDivElement).scrollIntoView?.();
            }
          }
        }
      }
    });

    const msgsBox = msgsBoxRef.current;
    if (msgsBox) {
      followingModeRef.current = determineFollowingMode(msgsBox);
    }

    if (msgsBoxRef.current) {
      mutObs.observe(msgsBoxRef.current, { childList: true });
      return () => mutObs.disconnect();
    }
  }, [activeConn]);

  const { saveScrollTop, restoreScrollTop } = useScrollTop();
  useLayoutEffect(() => {
    if (activeConn) {
      restoreScrollTop(activeConn, msgsBoxRef);
    }
  }, [activeConn]);

  const { updateUnreadMessageIds, getUnreadMessages, addUnreadMessageIds } =
    useUnreads();
  const unreads: string[] = getUnreadMessages()?.[nodeId] ?? [];

  useEffect(() => {
    const it = setInterval(
      () =>
        updateUnreadMessageIds(
          nodeIdRef.current,
          getVisibleMessageIds(msgsBoxRef),
        ),
      1000,
    );

    return () => clearInterval(it);
  }, []);

  const handleScroll = () => {
    updateUnreadMessageIds(nodeIdRef.current, getVisibleMessageIds(msgsBoxRef));

    const msgsBox = msgsBoxRef.current;
    if (msgsBox) {
      followingModeRef.current = determineFollowingMode(msgsBox);
    }
  };

  const onFileList = (filelist: FileList) => {
    if (!filelist || filelist.length <= 0) {
      return;
    }
    const fromNodeId = nodeIdRef.current;
    const toNodeId = activeConn;
    const pc = connTrackRef.current[activeConn]?.peerConnection;
    const chatDC = connTrackRef.current[activeConn]?.dataChannel;
    if (pc && chatDC) {
      for (const file of filelist) {
        let fileCat = ChatMessageFileCategory.Image;
        if (file.type.startsWith("video/")) {
          fileCat = ChatMessageFileCategory.Video;
        } else if (file.type.startsWith("image/")) {
          fileCat = ChatMessageFileCategory.Image;
        } else {
          fileCat = ChatMessageFileCategory.File;
        }
        if (
          fileCat === ChatMessageFileCategory.Image ||
          fileCat === ChatMessageFileCategory.Video
        ) {
          createThumbnailFromFile(file)
            .then((thumbnail) => {
              transmitFileViaPC(
                pc,
                chatDC,
                fromNodeId,
                toNodeId,
                fileCat,
                file,
                setConnTrackStatus,
                thumbnail,
              );
            })
            .catch((e) => {
              console.error(
                "failed to create thumbnail for file",
                file.name,
                e,
              );
            });
        } else {
          transmitFileViaPC(
            pc,
            chatDC,
            fromNodeId,
            toNodeId,
            fileCat,
            file,
            setConnTrackStatus,
            undefined,
          );
        }
      }
    }
  };
  const { showDropArea, onDrop, onDragOver, onMouseOut } =
    useFileDrop(onFileList);

  const handleDrawerToggle = () => {
    setMobileDrawerOpen(!mobileDrawerOpen);
  };

  const dispUsername =
    loggedInAs?.displayName ??
    loggedInAs?.username ??
    userPreferenceMap[nodeId ?? ""]?.name ??
    "";

  const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);
  const menuOpen = Boolean(menuAnchorEl);
  const handleCloseMenu = () => {
    setMenuAnchorEl(null);
  };

  const handleLogout = () => {
    logout(selectedserverObject?.apiPrefix || "");
    setPinnedServer("");
    clearLoggedInState();
    disconnect();
  };

  const drawerContent = (
    <Fragment>
      {selectedserverObject && pinnedServer ? (
        <Box>
          <Box
            sx={{
              display: "flex",
              flexDirection: "column",
              alignItems: "center",
              gap: 1,
              paddingTop: 4,
              paddingBottom: 0,
            }}
          >
            <RenderAvatar
              username={loggedInAs?.username ?? ""}
              size="large"
              preferredColorIdx={preference?.indexOfPreferColor}
            />
            <Box
              sx={{
                display: "flex",
                flexDirection: "row",
                alignItems: "center",
                gap: 0.5,
              }}
            >
              <Box>{dispUsername}</Box>
              <ConnStatusDisplay
                colorCodes={undefined}
                connStatus={wsConnStatus}
              />
              <Tooltip
                sx={{
                  marginLeft: -4,
                  position: "relative",
                  left: "30px",
                }}
                title="More"
              >
                <IconButton
                  size="small"
                  onClick={(event) => {
                    setMenuAnchorEl(event.currentTarget);
                  }}
                >
                  <Edit fontSize="small" />
                </IconButton>
              </Tooltip>
            </Box>
          </Box>
          <Box sx={{ padding: 2 }}>
            <TextField
              label="Search by name"
              value={searchKw}
              onChange={(e) => setSearchKw(e.target.value)}
              fullWidth
              variant="standard"
            />
          </Box>
          <Box>
            {sortConnsByLatestUnread(
              conns
                .filter((conn) => conn.node_id !== nodeId)
                .filter(
                  (conn) =>
                    !searchKw || conn.entry?.node_name?.includes(searchKw),
                ),
              connTrackStatus,
              new Set(unreads),
            ).map((conn) => {
              const unreadsSet = new Set(unreads);
              const unreadPeerMsgs = getPeerUnreadMsgs(
                connTrackStatus,
                conn.node_id,
                unreadsSet,
              );
              const preferredColorIdxS =
                conn.entry?.attributes?.[WellKnownAttributes.PreferredColor];
              const preferredColorIdx = !!preferredColorIdxS
                ? parseInt(preferredColorIdxS)
                : undefined;

              const numUnreads = unreadPeerMsgs.length;
              const latestUnreadMessage = unreadPeerMsgs.sort(
                (a, b) => b.timestamp - a.timestamp,
              )[0];
              return (
                <RenderPeerEntry
                  preferredColorIdx={preferredColorIdx}
                  conn={conn}
                  key={conn.node_id}
                  activeNodeId={activeConn}
                  onSelect={() => {
                    saveScrollTop(activeConn, msgsBoxRef);
                    if (selectedserverObject) {
                      switchActiveConn(
                        conn.node_id,
                        selectedserverObject.iceServers,
                        (msgIds) =>
                          addUnreadMessageIds(nodeIdRef.current, msgIds),
                      );
                    }
                    if (isMobile) {
                      setMobileDrawerOpen(false);
                    }
                  }}
                  numUnreads={numUnreads}
                  latestUnreadMessage={latestUnreadMessage}
                />
              );
            })}
          </Box>
        </Box>
      ) : (
        <ServerSelector
          onLogout={handleLogout}
          connecting={wsConnStatus === WSConnStatusShort.Connecting}
          preference={preference}
          onPreferenceChange={setPreference}
          onPinServer={(pinnedServer, preference) => {
            connect(pinnedServer, preference);
            setPinnedServer(pinnedServer.id);
          }}
          servers={servers}
        />
      )}
    </Fragment>
  );

  const topBar = (
    <Paper
      sx={{
        flexShrink: 0,
        padding: 1.5,
        borderRadius: 0,
        display: "flex",
        alignItems: "center",
        gap: 1,
      }}
    >
      {isMobile && (
        <IconButton onClick={handleDrawerToggle}>
          <MenuIcon />
        </IconButton>
      )}
      <RenderAvatar
        username={activeConn ? (userPreferenceMap[activeConn]?.name ?? "") : ""}
        size="small"
        preferredColorIdx={
          userPreferenceMap[activeConn]?.indexOfPreferColor ?? -1
        }
      />
      {activeConn && (
        <Box>
          <Box>{connTrackStatus[activeConn]?.rtt ?? ""}</Box>
          <Box>{connTrackStatus[activeConn]?.connectionStatus}</Box>
        </Box>
      )}

      <Box>{activeConn ? userPreferenceMap[activeConn]?.name : ""}</Box>
    </Paper>
  );

  return (
    <Fragment>
      <Box sx={{ display: "flex", flexDirection: "row", height: "100vh" }}>
        {isMobile ? (
          <Drawer
            anchor="left"
            open={mobileDrawerOpen}
            onClose={() => setMobileDrawerOpen(false)}
          >
            <LeftPanel>{drawerContent}</LeftPanel>
          </Drawer>
        ) : (
          <LeftPanel>{drawerContent}</LeftPanel>
        )}
        {activeConn ? (
          <Box
            sx={{
              flex: 1,
              minWidth: 0,
              display: "flex",
              flexDirection: "column",
              overflow: "hidden",
            }}
            onDrop={onDrop}
            onDragOver={onDragOver}
            onMouseOut={onMouseOut}
          >
            {topBar}
            {showDropArea ? (
              <Box
                sx={{
                  flex: 1,
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                }}
              >
                Release to drop
              </Box>
            ) : (
              <Fragment>
                <Box
                  ref={msgsBoxRef}
                  onScroll={handleScroll}
                  sx={{
                    flex: 1,
                    minHeight: 0,
                    overflow: "auto",
                    display: "flex",
                    flexDirection: "column",
                    gap: 1,
                    padding: 2,
                    position: "relative",
                  }}
                >
                  {messages.map((message) => (
                    <RenderMessage
                      key={message.messageId}
                      message={message}
                      patches={msgPatches[message.messageId] || []}
                      onAmend={(amendedMsg) => {
                        sendAmendMsg(amendedMsg);
                      }}
                      onDelete={(deletedMsgId) => {
                        sendMsgDeleteRequest(activeConn, deletedMsgId);
                      }}
                      fileTransferStatus={
                        connTrackStatus?.[activeConn]?.fileTransferStatus ?? {}
                      }
                      userPreferenceMap={userPreferenceMap}
                      audioContextRef={audioCtxRef}
                    />
                  ))}
                </Box>
                <Box sx={{ flexShrink: 0 }}>
                  <MessageComposer
                    disabled={!connTrackStatus[activeConn]?.readyToTalk}
                    supportAttachment={
                      conns.find((conn) => conn.node_id === activeConn)?.entry
                        ?.attributes?.[
                        WellKnownAttributes.SupportAttachment
                      ] === "true"
                    }
                    onFile={(filelist) => {
                      const fileCat = ChatMessageFileCategory.File;
                      const pc =
                        connTrackRef.current[activeConn]?.peerConnection;
                      const chatDC =
                        connTrackRef.current[activeConn]?.dataChannel;
                      const fromNodeId = nodeIdRef.current;
                      const toNodeId = activeConn;
                      if (filelist && filelist.length > 0 && pc && chatDC) {
                        for (const file of filelist) {
                          transmitFileViaPC(
                            pc,
                            chatDC,
                            fromNodeId,
                            toNodeId,
                            fileCat,
                            file,
                            setConnTrackStatus,
                            undefined,
                          );
                        }
                      }
                    }}
                    onPhoto={(filelist) => {
                      const pc =
                        connTrackRef.current[activeConn]?.peerConnection;
                      const chatDC =
                        connTrackRef.current[activeConn]?.dataChannel;
                      const fromNodeId = nodeIdRef.current;
                      const toNodeId = activeConn;
                      if (filelist && filelist.length > 0 && pc && chatDC) {
                        for (const file of filelist) {
                          let fileCat = ChatMessageFileCategory.Image;
                          if (file.type.startsWith("video/")) {
                            fileCat = ChatMessageFileCategory.Video;
                          }
                          createThumbnailFromFile(file)
                            .then((thumbnail) => {
                              transmitFileViaPC(
                                pc,
                                chatDC,
                                fromNodeId,
                                toNodeId,
                                fileCat,
                                file,
                                setConnTrackStatus,
                                thumbnail,
                              );
                            })
                            .catch((e) => {
                              console.error(
                                "failed to create thumbnail for file",
                                file.name,
                                e,
                              );
                            });
                        }
                      }
                    }}
                    onText={(text) => {
                      const dc = connTrackRef.current[activeConn]?.dataChannel;
                      if (dc) {
                        const msgObject: ChatMessage = {
                          messageId: crypto.randomUUID(),
                          message: text,
                          timestamp: Date.now(),
                          fromNodeId: nodeIdRef.current,
                          toNodeId: activeConn,
                        };
                        sendMsg(dc, msgObject, activeConn, setConnTrackStatus)
                          .then((msgObject) => {
                            console.log(
                              "[dbg] [ack] message",
                              msgObject.messageId,
                              "was acked",
                            );
                          })
                          .catch((e) => {
                            console.error("failed to send(or ack) message", e);
                          });
                      }
                    }}
                  />
                </Box>
              </Fragment>
            )}
          </Box>
        ) : (
          <Box
            sx={{
              flex: 1,
              display: "flex",
              flexDirection: "column",
            }}
          >
            {isMobile && (
              <Paper
                sx={{
                  flexShrink: 0,
                  padding: 1.5,
                  borderRadius: 0,
                  display: "flex",
                  alignItems: "center",
                  gap: 1,
                }}
              >
                <IconButton onClick={handleDrawerToggle}>
                  <MenuIcon />
                </IconButton>
              </Paper>
            )}
            <Box
              sx={{
                flex: 1,
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
              }}
            >
              Select a chat to start messaging
            </Box>
          </Box>
        )}
      </Box>
      <ChangePreference
        value={preference}
        onChange={setPreference}
        open={showPreferenceDialog}
        onClose={() => {
          setShowPreferenceDialog(false);
        }}
        onConfirm={(newPreference) => {
          return new Promise((resolve) => {
            const renamePayload: RenamePayload = {
              new_preference: newPreference,
              origin_node_name: name,
            };
            const renameMsg: MessagePayload = {
              rename: renamePayload,
            };
            sendWsMsg(renameMsg);

            resolve();
            setShowPreferenceDialog(false);
          });
        }}
      />
      <Menu
        anchorEl={menuAnchorEl}
        open={menuOpen}
        onClose={handleCloseMenu}
        slotProps={{
          list: {
            "aria-labelledby": "basic-button",
          },
        }}
      >
        <MenuItem
          onClick={() => {
            setPreference((prev) => ({
              ...prev,
              name: name || prev.name,
            }));
            setShowPreferenceDialog(true);
            handleCloseMenu();
          }}
        >
          Preference
        </MenuItem>
        <MenuItem
          onClick={() => {
            handleLogout();
            handleCloseMenu();
          }}
        >
          Logout
        </MenuItem>
      </Menu>
    </Fragment>
  );
}
