"use client";

import {
  ConnEntry,
  EchoDirectionC2S,
  EchoDirectionS2C,
  MessagePayload,
  RegisterPayload,
  RenamePayload,
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
import { Fragment, useEffect, useMemo, useRef, useState } from "react";
import { LeftPanel } from "@/components/LeftPanel";
import { getConns } from "@/apis/conns";

const googleStunServer = "stun:stun.l.google.com:19302";

const wsAddr = "ws://localhost:3001/ws";
const pingIntvMs = 1000;

function useWs() {
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
    conns,
    connected,
    connecting,
    wsRef,
    doConnect,
  };
}

function WSPanel(props: { wsUrl: string }) {
  const { wsUrl } = props;

  const [showChangeName, setShowChangeName] = useState(false);

  const {
    rtt,
    lastSeq,
    upTime,
    nodeId,

    conns,
    connected,
    connecting,
    wsRef,
    doConnect,
  } = useWs();

  const name = conns
    ? conns.find((conn) => conn.node_id === nodeId)?.entry?.node_name
    : undefined;
  const nameDisplay = name ? `as ${name}` : "";
  const [nameEdited, setNameEdited] = useState<string>("");
  const [activeConn, setActiveConn] = useState("");

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
                      setActiveConn(conn.node_id);
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

  const createPeerConnection = () => {
    setCandidates([]);
    const config = {
      iceServers: [{ urls: googleStunServer }],
    };
    const peerConnection = new RTCPeerConnection(config);
    peerConnection.oniceconnectionstatechange = (event) => {
      console.log("[dbg] ice connection state changed", event);
    };

    peerConnectionRef.current = peerConnection;

    // 2. Listen for local ICE candidates
    peerConnection.onicecandidate = (event) => {
      const candidate = event.candidate;
      if (candidate && candidate !== null) {
        // We found a candidate! Send it to the peer immediately.
        console.log("[dbg] new local ICE candidate:", candidate);
        setCandidates((prev) => [...prev, candidate]);

        // sendToSignalingServer({
        //   type: "candidate",
        //   candidate: event.candidate
        // });
      } else {
        // If event.candidate is null, it means the browser
        // has finished gathering all possible candidates.
        console.log("End of ICE candidate gathering.");
      }
    };

    peerConnection.ondatachannel = (event) => {
      console.log("[dbg] on data channel", event);
      const dc = event.channel;
      dataChannelRef.current = dc;
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

    return peerConnection;
  };

  useEffect(() => {
    const pc = createPeerConnection();
    return () => {
      pc.close();
    };
  }, []);
  const [myName, setMyName] = useState("user");

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
