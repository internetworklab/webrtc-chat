"use client";

import { WSServer } from "@/apis/types";
import { Box, TextField, Select, MenuItem, Button } from "@mui/material";
import { IaPLoginButton } from "./LoginButton";
import { Fragment } from "react";
import { PSKey, usePersistentStorage } from "@/apis/persistent";
import { useLoginStatusPolling } from "@/apis/profile";

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

  const { getValue: getLoggingIn, setValue: setLoggingIn } =
    usePersistentStorage(PSKey.LoggingIn);
  const isLoggingIn = getLoggingIn() === "true";

  const handleLoginClick = () => {
    // start polling (also the polling state would also survives page reload)
    setLoggingIn("true");

    // navigate the user to the oauth2 authorization portal
    if (hasIAP && selectedServerObj?.iap?.loginUrl) {
      window.open(selectedServerObj.iap.loginUrl);
    }
  };

  const selectedServerObj = servers.find(
    (server) => server.id === selectedServer,
  );
  const hasIAP = selectedServerObj?.iap && selectedServerObj.iap.loginUrl;
  const connectBtn = (
    <Button
      variant="contained"
      loading={connecting}
      onClick={() => {
        const server = servers.find((server) => server.id === selectedServer);
        if (server) {
          onConnect(server);
        }
      }}
    >
      Connect
    </Button>
  );

  const { loggedIn, loggedInAs, hintText } = useLoginStatusPolling(
    selectedServerObj?.apiPrefix || "",
    3000,
  );

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
        {!hasIAP ? (
          <Fragment>
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
            <Box
              sx={{
                display: "flex",
                justifyContent: "center",
                marginTop: "2",
                gridColumn: "1 / span 2",
              }}
            >
              {connectBtn}
            </Box>
          </Fragment>
        ) : (
          <Fragment>
            <Box
              sx={{
                display: "flex",
                justifyContent: "center",
                marginTop: "2",
                gridColumn: "1 / span 2",
              }}
            >
              {isLoggingIn && <Box>{hintText}</Box>}
              {loggedIn && loggedInAs ? (
                connectBtn
              ) : (
                <IaPLoginButton
                  loading={isLoggingIn}
                  onClick={handleLoginClick}
                  iapContext={selectedServerObj!.iap!}
                />
              )}
            </Box>
          </Fragment>
        )}
      </Box>
    </Box>
  );
}
