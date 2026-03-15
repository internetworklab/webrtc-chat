"use client";

import { IDProvider, Preference, Profile, WSServer } from "@/apis/types";
import {
  Box,
  TextField,
  Select,
  MenuItem,
  Button,
  useMediaQuery,
  useTheme,
  Divider,
} from "@mui/material";
import { IdPLoginButton } from "./LoginButton";
import { Dispatch, Fragment, SetStateAction } from "react";
import {
  getLoginStatusHintTxt,
  getProfile,
  getProfileStatus,
} from "@/apis/profile";
import { useQuery } from "@tanstack/react-query";
import { PSKey, usePersistentStorage } from "@/apis/persistent";

// Select what signalling server to use
export function ServerSelector(props: {
  servers: WSServer[];
  onPinServer: (
    server: WSServer,
    preference: Preference | undefined,
    hasLoggedIn: boolean,
  ) => void;
  connecting: boolean;
  onLogout: () => void;
  preference: Preference;
  onPreferenceChange: Dispatch<SetStateAction<Preference>>;
}) {
  const {
    servers,
    preference,
    onPreferenceChange,
    connecting,
    onPinServer,
    onLogout,
  } = props;

  const { getValue: getCurrentServer, setValue: setSelectedServer } =
    usePersistentStorage(PSKey.CurrentServer);

  // selectedServerId indicates the server that is currently active in the select box
  // the user might just selected a server, but didn't click the 'connect' button, so
  // the selectedServer might not necessarily be the pinnedSrv in the meantime
  const selectedServerId = getCurrentServer() || "";
  // pinnedSrv indicates which server the user decided to connect to

  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down("md"));

  const selectedServerObj = servers.find(
    (server) => server.id === selectedServerId,
  );
  const handleLoginClick = (idp: IDProvider) => {
    // eslint-disable-next-line
    window.location.href = idp.loginUrl;
  };

  const { isLoading: isLoginStatusLoading, data: profileStatusData } = useQuery(
    {
      queryKey: ["hasloggedin", selectedServerObj?.apiPrefix ?? ""],
      queryFn: () => getProfileStatus(selectedServerObj?.apiPrefix ?? ""),
    },
  );

  const { data: profileData, isLoading: isProfileDataLoading } = useQuery({
    queryKey: ["profile", selectedServerObj?.apiPrefix ?? ""],
    queryFn: () => getProfile(selectedServerObj?.apiPrefix ?? ""),
  });

  const hintText = getLoginStatusHintTxt(
    profileStatusData?.logged_in,
    profileData,
  );
  const idps = selectedServerObj?.idp ?? [];

  const handleConnect = () => {
    const server = servers.find((server) => server.id === selectedServerId);
    if (server && !isLoginStatusLoading) {
      // the app will automatically tries to connect to a pinned server
      onPinServer(server, preference, !!profileStatusData?.logged_in);
    }
  };

  const connectBtn = (
    <Button
      fullWidth
      variant="contained"
      loading={connecting}
      onClick={handleConnect}
    >
      Connect
    </Button>
  );

  return (
    <Box
      sx={{
        padding: 1,
        display: "flex",
        flexDirection: "column",
        justifyContent: "center",
        height: "100%",
        gap: 2,
      }}
    >
      <Box
        sx={{
          display: "flex",
          alignItems: "center",
          flexWrap: "wrap",
          rowGap: 0,
          columnGap: 2,
        }}
      >
        {!isMobile && <Box>{"Server:"}</Box>}
        <Box sx={{ flex: "1" }}>
          <Select
            fullWidth
            variant="standard"
            label="Server"
            value={selectedServerId}
            onChange={(e) => setSelectedServer(e.target.value)}
          >
            {servers.map((server) => (
              <MenuItem key={server.id} value={server.id}>
                {server.name}
              </MenuItem>
            ))}
          </Select>
        </Box>
      </Box>

      {selectedServerObj && (
        <Fragment>
          <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
            {isLoginStatusLoading ? (
              <Box>Fetching login status ...</Box>
            ) : profileStatusData?.logged_in ? (
              <Fragment>
                <Box>{hintText}</Box>
                <Box>{connectBtn}</Box>
                <Box>
                  <Button variant="outlined" fullWidth onClick={onLogout}>
                    Logout
                  </Button>
                </Box>
              </Fragment>
            ) : (
              <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
                {idps.map((idp) => (
                  <IdPLoginButton
                    key={idp.name}
                    idpContext={idp}
                    onClick={() => handleLoginClick(idp)}
                  />
                ))}
              </Box>
            )}
          </Box>
          {selectedServerObj?.allowAnonymous &&
            !profileStatusData?.logged_in && (
              <Box
                sx={{
                  display: "flex",
                  flexDirection: "column",
                  gap: 2,
                }}
              >
                <Box>Or, Connect as a visitor:</Box>
                <TextField
                  placeholder="Pick a username"
                  fullWidth
                  variant="standard"
                  value={preference?.name ?? ""}
                  onChange={(e) =>
                    onPreferenceChange((prev) => {
                      return {
                        ...prev,
                        name: e.target.value,
                      };
                    })
                  }
                  onKeyDown={(e) => {
                    if (e.key === "Enter") {
                      e.preventDefault();
                      e.stopPropagation();
                      handleConnect();
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
              </Box>
            )}
        </Fragment>
      )}
    </Box>
  );
}
