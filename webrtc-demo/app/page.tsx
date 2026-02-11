"use client";

import {
  EchoDirectionC2S,
  EchoDirectionS2C,
  MessagePayload,
  RegisterPayload,
} from "@/apis/types";
import { ContentCopy, CopyAll, Refresh } from "@mui/icons-material";
import {
  Box,
  Button,
  Card,
  Chip,
  Dialog,
  DialogActions,
  DialogTitle,
  DialogContent,
  TextField,
  Tooltip,
  IconButton,
  Paper,
} from "@mui/material";
import { Fragment, useEffect, useMemo, useRef, useState } from "react";

const googleStunServer = "stun:stun.l.google.com:19302";

function MultipleInputAcceptor(props: {
  title: string;
  rows?: number;
  open: boolean;
  onCancel: () => void;
  onConfirm: (input: string) => void;
}) {
  const [candidateText, setCandidateText] = useState("");
  const { title, rows = 4, open, onCancel, onConfirm } = props;
  return (
    <Dialog
      maxWidth="md"
      fullWidth
      open={open}
      onClose={() => {
        onCancel();
      }}
    >
      <DialogTitle>{title}</DialogTitle>
      <DialogContent>
        <TextField
          variant="outlined"
          multiline
          rows={rows}
          fullWidth
          value={candidateText}
          onChange={(e) => {
            setCandidateText(e.target.value);
          }}
        />
        <DialogActions sx={{ marginTop: 2 }}>
          <Button
            onClick={() => {
              onCancel();
            }}
          >
            Cancel
          </Button>
          <Button
            variant="contained"
            onClick={() => {
              onConfirm(candidateText);
            }}
          >
            Set
          </Button>
        </DialogActions>
      </DialogContent>
    </Dialog>
  );
}

function CandidateInputDialog(props: {
  peerConnectionRef: React.RefObject<RTCPeerConnection | null>;
  open: boolean;
  onClose: () => void;
}) {
  const { peerConnectionRef, open, onClose } = props;
  return (
    <MultipleInputAcceptor
      title="Add Candidate"
      rows={4}
      open={open}
      onCancel={onClose}
      onConfirm={(input) => {
        try {
          const candidate = JSON.parse(input);
          peerConnectionRef.current?.addIceCandidate(candidate);
          onClose();
        } catch (e) {
          console.error(e);
        }
      }}
    />
  );
}

function RemoteDescriptionInputDialog(props: {
  peerConnectionRef: React.RefObject<RTCPeerConnection | null>;
  open: boolean;
  onClose: () => void;
}) {
  const { peerConnectionRef, open, onClose } = props;
  return (
    <MultipleInputAcceptor
      title="Remote Description"
      rows={4}
      open={open}
      onCancel={onClose}
      onConfirm={(input) => {
        try {
          const remoteDescription = JSON.parse(input);
          peerConnectionRef.current?.setRemoteDescription(remoteDescription);
          onClose();
        } catch (e) {
          console.error(e);
        }
      }}
    />
  );
}

function LocalDescriptionInputDialog(props: {
  peerConnectionRef: React.RefObject<RTCPeerConnection | null>;
  open: boolean;
  onClose: () => void;
}) {
  const { peerConnectionRef, open, onClose } = props;
  return (
    <MultipleInputAcceptor
      title="Local Description"
      rows={4}
      open={open}
      onCancel={onClose}
      onConfirm={(input) => {
        try {
          const localDescription = JSON.parse(input);
          peerConnectionRef.current?.setLocalDescription(localDescription);
          onClose();
        } catch (e) {
          console.error(e);
        }
      }}
    />
  );
}

function OfferDialog(props: {
  open: boolean;
  onClose: () => void;
  peerConnectionRef: React.RefObject<RTCPeerConnection | null>;
  dataChannelRef: React.RefObject<RTCDataChannel | null>;
}) {
  const [candidateText, setCandidateText] = useState("");
  const { open, onClose, peerConnectionRef, dataChannelRef } = props;
  return (
    <Dialog
      maxWidth="md"
      fullWidth
      open={open}
      onClose={() => {
        onClose();
      }}
    >
      <DialogTitle
        sx={{
          display: "flex",
          flexDirection: "row",
          gap: 1,
          alignItems: "center",
          justifyContent: "space-between",
          flexWrap: "wrap",
        }}
      >
        <Box>Offer</Box>
        <Box>
          <Tooltip title={"Refresh"}>
            <IconButton
              onClick={() => {
                const peerConnection = peerConnectionRef.current;
                const dc = peerConnection?.createDataChannel("dc1");
                if (dc) {
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

                  peerConnectionRef.current?.createOffer().then((offer) => {
                    setCandidateText(JSON.stringify(offer));
                    peerConnection?.setLocalDescription(offer);
                  });
                }
              }}
            >
              <Refresh />
            </IconButton>
          </Tooltip>
          <Tooltip title={"Copy"}>
            <IconButton
              onClick={() => {
                navigator?.clipboard?.writeText(candidateText);
              }}
            >
              <ContentCopy />
            </IconButton>
          </Tooltip>
        </Box>
      </DialogTitle>
      <DialogContent>
        <TextField
          variant="outlined"
          multiline
          rows={4}
          fullWidth
          value={candidateText}
          onChange={(e) => {
            setCandidateText(e.target.value);
          }}
        />
      </DialogContent>
    </Dialog>
  );
}

type AnswerDlgState = {
  show: boolean;
  answer: RTCSessionDescriptionInit | null;
};

function AnswerDialog(props: {
  open: boolean;
  onClose: () => void;
  peerConnectionRef: React.RefObject<RTCPeerConnection | null>;
}) {
  const { open, onClose, peerConnectionRef } = props;
  const [answerText, setAnswerText] = useState("");
  return (
    <Dialog
      maxWidth="md"
      fullWidth
      open={open}
      onClose={() => {
        onClose();
      }}
    >
      <DialogTitle
        sx={{
          display: "flex",
          flexDirection: "row",
          gap: 1,
          alignItems: "center",
          justifyContent: "space-between",
          flexWrap: "wrap",
        }}
      >
        <Box>Answer</Box>
        <Box>
          <Tooltip title={"Refresh"}>
            <IconButton
              onClick={() => {
                peerConnectionRef.current?.createAnswer().then((answer) => {
                  setAnswerText(JSON.stringify(answer));
                });
              }}
            >
              <Refresh />
            </IconButton>
          </Tooltip>
          <Tooltip title={"Copy"}>
            <IconButton
              onClick={() => {
                navigator?.clipboard?.writeText(answerText);
              }}
            >
              <ContentCopy />
            </IconButton>
          </Tooltip>
        </Box>
      </DialogTitle>
      <DialogContent>
        <TextField
          variant="outlined"
          multiline
          rows={4}
          fullWidth
          value={answerText}
        />
      </DialogContent>
    </Dialog>
  );
}

const wsAddr = "ws://localhost:3001/ws";
const pingIntvMs = 1000;

function WSPanel(props: {
  wsUrl: string;
  myName: string;
  onNameChange: (name: string) => void;
}) {
  const { wsUrl, myName, onNameChange } = props;
  const [connected, setConnected] = useState(false);
  const [connecting, setConnecting] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const correlationId = useMemo(() => crypto.randomUUID(), []);

  const [showChangeName, setShowChangeName] = useState(false);
  const [nodeId, setNodeId] = useState<string>("");
  const seqRef = useRef(0);
  const pingTxMapRef = useRef<Record<string, number>>({});
  const [rtt, setRtt] = useState<number | undefined>(undefined);
  const [lastSeq, setLastSeq] = useState<number | undefined>(undefined);
  const connectedAtRef = useRef<number | undefined>(undefined);
  const [upTime, setUpTime] = useState<number | undefined>(undefined);

  const doConnect = (addr: string) => {
    setConnecting(true);
    const ws = new WebSocket(addr);
    wsRef.current = ws;

    let it: ReturnType<typeof setInterval> | undefined = undefined;

    ws.onopen = () => {
      connectedAtRef.current = Date.now();
      setConnected(true);
      setConnecting(false);
      console.log("[dbg] ws connected", ws);
      const registerPayload: RegisterPayload = {
        node_name: myName,
      };
      const registerMsg: MessagePayload = {
        register: registerPayload,
      };
      ws.send(JSON.stringify(registerMsg));

      it = setInterval(() => {
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
    ws.onclose = () => {
      setConnected(false);
      setConnecting(false);
    };
    ws.onerror = (error) => {
      console.error("[dbg] ws error", error);
      setConnecting(false);
    };
    ws.onmessage = (event) => {
      try {
        const msg: MessagePayload = JSON.parse(event.data);
        if (msg.node_id) {
          setNodeId(msg.node_id);
        }
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
          }
        }
      } catch (e) {
        console.error("[dbg] ws message error", e);
      }
    };
  };

  return (
    <Fragment>
      <Box>
        {connected ? (
          <Box>
            <Box>
              Connected to {wsRef?.current?.url} as {myName}
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
                  setShowChangeName(true);
                }}
              >
                Change Name
              </Button>
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
    </Fragment>
  );
}

function LeftPanel(props: { initW?: number; children: React.ReactNode }) {
  const { initW = 360, children } = props;
  const [width, setWidth] = useState(initW);
  return (
    <Box sx={{ width: `${width}px`, height: "100%", position: "relative" }}>
      <Paper sx={{ height: "100%" }}>{children}</Paper>
      <Box
        onMouseDown={(e) => {
          const w0 = width;
          const x0 = e.clientX;
          const handleMouseMove = (e: MouseEvent) => {
            const x = e.clientX;
            const dw = x - x0;
            const w = w0 + dw;
            setWidth(w);
          };
          window.addEventListener("mousemove", handleMouseMove);
          const handleMouseUp = () => {
            window.removeEventListener("mousemove", handleMouseMove);
            window.removeEventListener("mouseup", handleMouseUp);
          };
          window.addEventListener("mouseup", handleMouseUp);
        }}
        sx={{
          position: "absolute",
          top: 0,
          right: 0,
          width: "6px",
          backgroundColor: "#aaa",
          height: "100%",
          cursor: "col-resize",
          opacity: 0,
          transition: "opacity 0.3s ease-in-out",
          "&:hover": {
            opacity: 1,
          },
        }}
      ></Box>
    </Box>
  );
}

export default function Home() {
  useEffect(() => {}, []);

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
          <Box sx={{ padding: 2 }}>
            <WSPanel myName={myName} wsUrl={wsAddr} onNameChange={setMyName} />
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
