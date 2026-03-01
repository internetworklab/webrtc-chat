"use client";

import { WSServer } from "@/apis/types";
import { Box, TextField, Select, MenuItem, Button } from "@mui/material";

// Select what signalling server to use

export function ServerSelector(props: {
  servers: WSServer[];
  onConnect: (server: WSServer) => void;
  selectedServer: string;
  onSelectedServerChange: (serverId: string) => void;
  preferName: string;
  onPreferNameChange: (preferName: string) => void;
  connecting: boolean;
}) {
  const {
    servers,
    selectedServer,
    onSelectedServerChange,
    onConnect,
    preferName,
    onPreferNameChange,
    connecting,
  } = props;
  return (
    <Box
      sx={{
        display: "flex",
        flexDirection: "column",
        justifyContent: "center",
        height: "100%",
      }}
    >
      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: "auto 1fr",
          gap: 1,
          rowGap: 2,
          alignItems: "center",
          padding: 2,
        }}
      >
        <Box sx={{ justifySelf: "right" }}>Choose Server:</Box>
        <Select
          variant="standard"
          label="Server"
          value={selectedServer}
          onChange={(e) => onSelectedServerChange(e.target.value)}
        >
          {servers.map((server) => (
            <MenuItem key={server.id} value={server.id}>
              {server.name}
            </MenuItem>
          ))}
        </Select>
        <Box sx={{ justifySelf: "right" }}>Pick a Name:</Box>
        <TextField
          fullWidth
          variant="standard"
          value={preferName}
          onChange={(e) => onPreferNameChange(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              e.stopPropagation();
              const server = servers.find(
                (server) => server.id === selectedServer,
              );
              if (server) {
                onConnect(server);
              }
            }
          }}
        />
      </Box>
      <Box
        sx={{
          display: "flex",
          justifyContent: "center",
          marginTop: 2,
        }}
      >
        <Button
          variant="contained"
          loading={connecting}
          onClick={() => {
            const server = servers.find(
              (server) => server.id === selectedServer,
            );
            if (server) {
              onConnect(server);
            }
          }}
        >
          Connect
        </Button>
      </Box>
    </Box>
  );
}
