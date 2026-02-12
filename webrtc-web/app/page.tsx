"use client";

import {
  ConnEntry,
  EchoDirectionC2S,
  EchoDirectionS2C,
  ICEOfferPayload,
  MessagePayload,
  OfferType,
  RegisterPayload,
  RenamePayload,
  SDPOfferPayload,
} from "@/apis/types";
import {
  AnswerDialog,
  CandidateInputDialog,
  ChangeNameDialog,
  LocalDescriptionInputDialog,
  OfferDialog,
  RemoteDescriptionInputDialog,
} from "@/components/InputDialog";
import { Box, Button, MenuItem, Typography } from "@mui/material";
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
import { RenderICECandidate } from "@/components/RenderICECandidate";
import { BasicWsInfo } from "@/components/BasicWsInfo";
import { RenderPeerEntry } from "@/components/RenderPeerEntry";

const googleStunServer = "stun:stun.l.google.com:19302";

const wsAddr = "ws://localhost:3001/ws";
const pingIntvMs = 1000;

type ChatMessage = {
  // message uuid, globally unique, to prevent a message from being queued multiple times.
  messageId: string;
  fromNodeId?: string;
  toNodeId?: string;
  message: string;
  messageMIME?: string;
  timestamp: number;
};
type ConnTrackStatusEntry = {
  // todo
  disconnected?: boolean;
  connecting?: boolean;
  messages?: ChatMessage[];
};
// key is the node_id of remote peer
type ConnTrackStatus = Record<string, ConnTrackStatusEntry>;

type ConnTrackEntry = {
  peerConnection: RTCPeerConnection;
  remoteOffers: RTCSessionDescriptionInit[];
  queuedICEOffers: RTCIceCandidateInit[];
  dataChannel?: RTCDataChannel | null;
};

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

            ent.peerConnection.ondatachannel = (event) => {
              console.log(`[dbg] [${logSource}] on data channel`, event);
              const dc = event.channel;
              ent.dataChannel = dc;
              attachDCEventListeners(
                dc,
                setConnTrackStatus,
                remoteNodeId,
                logSource,
              );
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
                "[dbg] adding queued ICE offers to peer connection",
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

function attachDCEventListeners(
  dc: RTCDataChannel,
  setConnTrackStatus: Dispatch<SetStateAction<ConnTrackStatus>>,
  remoteNodeId: string,
  logId?: string,
) {
  const logSource = logId ? ` [${logId}]` : "";
  dc.onopen = () => {
    console.log(`[dbg]${logSource} data channel opened`, dc);
  };

  dc.onclose = () => {
    console.log(`[dbg]${logSource} data channel closed`, dc);
  };

  dc.onerror = (error) => {
    console.error(`[dbg]${logSource} data channel error`, error);
  };

  dc.onmessage = (event) => {
    console.log(`[dbg]${logSource} data channel message`, event.data, dc);
    try {
      const msgObject: ChatMessage = JSON.parse(event.data);
      setConnTrackStatus((prev) => {
        const theEntry = prev[remoteNodeId] ? { ...prev[remoteNodeId] } : {};
        const messages = theEntry.messages ? [...theEntry.messages] : [];
        const idx = messages.findIndex(
          (msg) => msg.messageId === msgObject.messageId,
        );
        if (idx === -1) {
          messages.push(msgObject);
        } else {
          console.log(
            `[dbg]${logSource}`,
            "message",
            msgObject,
            "is already in the queue, skipping",
          );
        }
        theEntry.messages = messages;
        return {
          ...prev,
          [remoteNodeId]: theEntry,
        };
      });
    } catch (e) {
      console.error("failed to parse data channel message", e);
    }

    console.log(`[dbg]${logSource} data channel message`, event.data, dc);
  };
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
  const [candidates, setCandidates] = useState<RTCIceCandidate[]>([]);
  const [addCandidateDlgOpen, setAddCandidateDlgOpen] = useState(false);
  const [setRemoteDescriptionDlgOpen, setSetRemoteDescriptionDlgOpen] =
    useState(false);
  const [setLocalDescriptionDlgOpen, setSetLocalDescriptionDlgOpen] =
    useState(false);
  const [offerDlgOpen, setOfferDlgOpen] = useState(false);
  const peerConnectionRef = useRef<RTCPeerConnection | null>(null);
  const dataChannelRef = useRef<RTCDataChannel | null>(null);
  const [showAnswerDlg, setShowAnswerDlg] = useState<boolean>(false);

  // todo: display it in GUI
  const [connTrackStatus, setConnTrackStatus] = useState<ConnTrackStatus>({});
  console.log("[dbg] connTrackStatus", connTrackStatus);

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

      const dc = ent.peerConnection.createDataChannel("dc1");
      ent.dataChannel = dc;

      attachDCEventListeners(dc, setConnTrackStatus, logSource);

      console.log(
        `[dbg] [${logSource}] creating SDP offer to remote peer`,
        remoteNodeId,
      );
      ent.peerConnection
        .createOffer()
        .then((offer) => {
          console.log(
            `[dbg] [${logSource}] offer created`,
            offer,
            "sending offer to remote peer",
            remoteNodeId,
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
            `[dbg] [${logSource}] failed to create offer to remote peer`,
            remoteNodeId,
            e,
          );
        });
    }
  };

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
                      {conns.map((conn) => (
                        <RenderPeerEntry
                          conn={conn}
                          key={conn.node_id}
                          activeNodeId={activeConn}
                          onSelect={() => switchActiveConn(conn.node_id)}
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
            padding: 2,
            flex: 1,
            display: "flex",
            flexDirection: "column",
            gap: 1,
          }}
        >
          {candidates
            .filter((candidate) => !!candidate.address)
            .map((candidate) => (
              <RenderICECandidate
                candidate={candidate}
                key={`addr=${candidate.address},type=${candidate.type},protocol=${candidate.protocol}`}
              />
            ))}
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
