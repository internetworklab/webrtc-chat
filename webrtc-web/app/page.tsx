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
import { Box, Button, Card, Chip, MenuItem, Typography } from "@mui/material";
import {
  Dispatch,
  Fragment,
  SetStateAction,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { LeftPanel } from "@/components/LeftPanel";
import { getConns } from "@/apis/conns";

const googleStunServer = "stun:stun.l.google.com:19302";

const wsAddr = "ws://localhost:3001/ws";
const pingIntvMs = 1000;

type ConnTrackStatusEntry = {
  // todo
  disconnected?: boolean;
  connecting?: boolean;
};
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
            "[dbg] got SDP offer of type",
            msg.sdp_offer.type,
            "from remote peer",
            msg.sdp_offer,
          );
          const remoteNodeId = msg.sdp_offer.from_node_id;
          if (!(remoteNodeId in connTrackRef.current)) {
            // automatically accept the SDP offer
            console.log(
              "[dbg] automatically accepting SDP offer from remote peer",
              remoteNodeId,
            );
            const ent = makeConnTrackEntry();
            connTrackRef.current[remoteNodeId] = ent;
            console.log(
              "[dbg] initializing conn track entry for remote peer",
              remoteNodeId,
              ent,
            );

            // registering event handlers for peerconnection handle
            ent.peerConnection.oniceconnectionstatechange = (event) => {
              console.log(
                "[dbg] [onacceptor] ice connection state changed",
                event,
              );

              if (ent.peerConnection.iceConnectionState === "failed") {
                // see: https://developer.mozilla.org/en-US/docs/Web/API/WebRTC_API/Session_lifetime#ice_restart
                // todo:  pc.setConfiguration(restartConfig);
                console.log(
                  "[dbg] [onacceptor] ice connection to peer",
                  remoteNodeId,
                  "state changed to failed, doing ICE-restart after 1 second",
                );
                setTimeout(() => {
                  console.log(
                    "[dbg] [onacceptor] restarting ICE for peer",
                    remoteNodeId,
                  );
                  ent.peerConnection.restartIce();

                  console.log(
                    "[dbg] [onacceptor] creating iceRestart offer for peer",
                    remoteNodeId,
                  );
                  ent.peerConnection
                    .createOffer({ iceRestart: true })
                    .then((offer) => {
                      console.log(
                        "[dbg] [onacceptor] iceRestart offer created for peer",
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
                        "[dbg] [onacceptor] failed to create iceRestart offer for peer",
                        remoteNodeId,
                        e,
                      );
                    });
                }, 1000);
              }
            };

            ent.peerConnection.onicecandidate = (event) => {
              const candidate = event.candidate;
              if (candidate && candidate !== null) {
                // We found a candidate! Send it to the peer immediately.
                console.log("[dbg] new local ICE candidate:", candidate);

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
                console.log("[dbg] End of ICE candidate gathering.");
              }
            };

            ent.peerConnection.ondatachannel = (event) => {
              console.log("[dbg] [passive] on data channel", event);
              const dc = event.channel;
              ent.dataChannel = dc;
              dc.onopen = () => {
                console.log("[dbg] data channel opened", dc);
              };
              dc.onclose = () => {
                console.log("[dbg] data channel closed", dc);
              };
              dc.onerror = (error) => {
                console.error("[dbg] data channel error", error);
              };
              dc.onmessage = (event) => {
                console.log("[dbg] data channel message", event.data, dc);
              };
            };
          }
          const entry = connTrackRef.current[remoteNodeId];
          try {
            const offer = JSON.parse(msg.sdp_offer.offer_json);
            entry.remoteOffers.push(offer);
            entry.peerConnection.setRemoteDescription(offer);
            if (msg.sdp_offer.type === OfferType.Offer) {
              console.log(
                "[dbg] creating answer for SDP offer from remote peer",
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

function WSPanel(props: { wsUrl: string }) {
  const { wsUrl } = props;

  const [showChangeName, setShowChangeName] = useState(false);

  // todo
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
  const nameDisplay = name ? `as ${name}` : "";
  const [nameEdited, setNameEdited] = useState<string>("");
  const [activeConn, setActiveConn] = useState("");
  const switchActiveConn = (remoteNodeId: string) => {
    setActiveConn(remoteNodeId);

    let ent = connTrackRef.current[remoteNodeId];
    if (!ent) {
      ent = makeConnTrackEntry();
      connTrackRef.current[remoteNodeId] = ent;
      console.log(
        "[dbg] initializing conn track entry for remote peer",
        remoteNodeId,
        ent,
      );

      // registering event handlers for peerconnection handle
      ent.peerConnection.oniceconnectionstatechange = (event) => {
        console.log("[dbg] ice connection state changed", event);
        if (ent.peerConnection.iceConnectionState === "failed") {
          // see: https://developer.mozilla.org/en-US/docs/Web/API/WebRTC_API/Session_lifetime#ice_restart
          // todo:  pc.setConfiguration(restartConfig);
          console.log(
            "[dbg] ice connection to peer",
            remoteNodeId,
            "state changed to disconnected or failed, doing ICE-restart after 1 second",
          );
          setTimeout(() => {
            console.log("[dbg] restarting ICE for peer", remoteNodeId);
            ent.peerConnection.restartIce();

            console.log(
              "[dbg] creating iceRestart offer for peer",
              remoteNodeId,
            );
            ent.peerConnection
              .createOffer({ iceRestart: true })
              .then((offer) => {
                console.log(
                  "[dbg] iceRestart offer created for peer",
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
                  "[dbg] failed to create iceRestart offer for peer",
                  remoteNodeId,
                  e,
                );
              });
          }, 1000);
        }
      };

      ent.peerConnection.onicecandidate = (event) => {
        const candidate = event.candidate;
        if (candidate && candidate !== null) {
          // We found a candidate! Send it to the peer immediately.
          console.log("[dbg] new local ICE candidate:", candidate);

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
          console.log("[dbg] End of ICE candidate gathering.");
        }
      };

      ent.peerConnection.ondatachannel = (event) => {
        console.log("[dbg] [src1] [active] on data channel", event);
        const dc = event.channel;
        ent.dataChannel = dc;
        dc.onopen = () => {
          console.log("[dbg] [src1] data channel opened", dc);
        };
        dc.onclose = () => {
          console.log("[dbg] [src1] data channel closed", dc);
        };
        dc.onerror = (error) => {
          console.error("[dbg] [src1] data channel error", error);
        };
        dc.onmessage = (event) => {
          console.log("[dbg] [src1] data channel message", event.data, dc);
        };
      };

      const dc = ent.peerConnection.createDataChannel("dc1");
      ent.dataChannel = dc;
      dc.onopen = () => {
        console.log("[dbg] [src2] data channel opened", dc);
      };
      dc.onclose = () => {
        console.log("[dbg] [src2] data channel closed", dc);
      };
      dc.onerror = (error) => {
        console.error("[dbg] [src2] data channel error", error);
      };
      dc.onmessage = (event) => {
        console.log("[dbg] [src2] data channel message", event.data, dc);
      };
      console.log("[dbg] [src2] creating offer to remote peer", remoteNodeId);
      ent.peerConnection
        .createOffer()
        .then((offer) => {
          console.log(
            "[dbg] [src2] offer created",
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
            "[dbg] [src2] failed to create offer to remote peer",
            remoteNodeId,
            e,
          );
        });
    }
  };

  return (
    <Fragment>
      <Box>
        {connected ? (
          <Box>
            <Box sx={{ padding: 2 }}>
              <Box>Basics Info</Box>
              <Box>
                Connected to {wsRef?.current?.url} {nameDisplay}
              </Box>
              {nodeId && <Box>NodeId: {nodeId}</Box>}

              {rtt !== undefined && <Box>RTT: {rtt}ms</Box>}
              {lastSeq !== undefined && <Box>Last Seq: {lastSeq}</Box>}
              {upTime !== undefined && (
                <Box>
                  Up Time:{" "}
                  {(upTime / 1000)
                    .toFixed(3)
                    .replace(/0+$/, "")
                    .replace(/\.$/, "")}
                  s
                </Box>
              )}

              <Box>
                <Button
                  onClick={() => {
                    setNameEdited(name ?? "");
                    setShowChangeName(true);
                  }}
                >
                  Change Name
                </Button>
              </Box>
            </Box>
            <Box>
              <Box sx={{ padding: 2 }}>Peers</Box>
              <Box>
                {conns.map((conn) => (
                  <MenuItem
                    selected={activeConn === conn.node_id}
                    onClick={() => {
                      switchActiveConn(conn.node_id);
                    }}
                    key={conn.node_id}
                    sx={{ overflow: "hidden" }}
                  >
                    {conn.entry?.node_name || conn.node_id}
                    <Typography
                      component="span"
                      variant="body2"
                      gutterBottom={false}
                      marginLeft={1}
                      noWrap
                    >
                      {conn.node_id}
                    </Typography>
                  </MenuItem>
                ))}
              </Box>
            </Box>
          </Box>
        ) : (
          <Box>
            <Button
              loading={connecting}
              onClick={() => {
                doConnect(wsUrl);
              }}
            >
              Connect
            </Button>
          </Box>
        )}
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

  return (
    <Fragment>
      <Box sx={{ display: "flex", flexDirection: "row", height: "100vh" }}>
        <LeftPanel>
          <Box>
            <WSPanel wsUrl={wsAddr} />
          </Box>
        </LeftPanel>
        <Box sx={{ padding: 2, flex: 1 }}>
          <Box
            sx={{
              display: "flex",
              flexDirection: "row",
              gap: 1,
              flexWrap: "wrap",
            }}
          >
            <Button
              onClick={() => {
                setSetRemoteDescriptionDlgOpen(true);
              }}
            >
              Set Remote Description
            </Button>
            <Button
              onClick={() => {
                setSetLocalDescriptionDlgOpen(true);
              }}
            >
              Set Local Description
            </Button>
            <Button
              onClick={() => {
                setAddCandidateDlgOpen(true);
              }}
            >
              Add Candidate
            </Button>
            <Button
              onClick={() => {
                setOfferDlgOpen(true);
              }}
            >
              Offer
            </Button>
            <Button
              onClick={() => {
                setShowAnswerDlg(true);
              }}
            >
              Answer
            </Button>
          </Box>
          <Box
            sx={{
              marginTop: 2,
            }}
          >
            <Box
              sx={{
                flex: 1,
                flexShrink: 0,
                display: "flex",
                flexDirection: "column",
                gap: 1,
              }}
            >
              {candidates
                .filter((candidate) => !!candidate.address)
                .map((candidate) => (
                  <Card
                    sx={{
                      padding: 2,
                      display: "flex",
                      flexDirection: "row",
                      gap: 1,
                      flexWrap: "wrap",
                      justifyContent: "space-between",
                      alignItems: "center",
                    }}
                    key={`addr=${candidate.address},type=${candidate.type},protocol=${candidate.protocol}`}
                  >
                    <Box>
                      <Box
                        sx={{
                          display: "flex",
                          flexDirection: "row",
                          gap: 1,
                          flexWrap: "wrap",
                        }}
                      >
                        <Chip label={`Type: ${candidate.type}`} />
                        <Chip label={`Protocol: ${candidate.protocol}`} />
                        <Chip label={`Component: ${candidate.component}`} />
                        <Chip label={`Port: ${candidate.port}`} />
                      </Box>
                      <Box sx={{ marginTop: 1 }}>
                        <Box>Address: {candidate.address}</Box>
                      </Box>
                    </Box>
                    <Button
                      onClick={() => {
                        navigator?.clipboard?.writeText(
                          JSON.stringify(candidate.toJSON()),
                        );
                      }}
                    >
                      Copy
                    </Button>
                  </Card>
                ))}
            </Box>
          </Box>
        </Box>
      </Box>

      <CandidateInputDialog
        peerConnectionRef={peerConnectionRef}
        open={addCandidateDlgOpen}
        onClose={() => {
          setAddCandidateDlgOpen(false);
        }}
      />
      <RemoteDescriptionInputDialog
        peerConnectionRef={peerConnectionRef}
        open={setRemoteDescriptionDlgOpen}
        onClose={() => {
          setSetRemoteDescriptionDlgOpen(false);
        }}
      />
      <LocalDescriptionInputDialog
        peerConnectionRef={peerConnectionRef}
        open={setLocalDescriptionDlgOpen}
        onClose={() => {
          setSetLocalDescriptionDlgOpen(false);
        }}
      />
      <OfferDialog
        peerConnectionRef={peerConnectionRef}
        dataChannelRef={dataChannelRef}
        open={offerDlgOpen}
        onClose={() => {
          setOfferDlgOpen(false);
        }}
      />
      <AnswerDialog
        open={showAnswerDlg}
        onClose={() => {
          setShowAnswerDlg(false);
        }}
        peerConnectionRef={peerConnectionRef}
      />
    </Fragment>
  );
}
