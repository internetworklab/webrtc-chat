"use client";

import { ColorToken } from "@/apis/colors";
import { WSConnStatusShort } from "@/apis/types";
import { Box, Tooltip } from "@mui/material";

type ColorCode = Record<string, ColorToken>;

const defaultColorCodes: ColorCode = {
  [WSConnStatusShort.Online]: { light: "#4caf50", dark: "#4caf50" },
  [WSConnStatusShort.Connecting]: { light: "#ff9800", dark: "#ff9800" },
  [WSConnStatusShort.Offline]: { light: "#f44336", dark: "#f44336" },
  [WSConnStatusShort.Unknown]: { light: "#9e9e9e", dark: "#9e9e9e" },
};

const statusLabels: Record<WSConnStatusShort, string> = {
  [WSConnStatusShort.Online]: "Online",
  [WSConnStatusShort.Connecting]: "Connecting",
  [WSConnStatusShort.Offline]: "Disconnected",
  [WSConnStatusShort.Unknown]: "Unknown",
};

export function ConnStatusDisplay(props: {
  connStatus: WSConnStatusShort | undefined;
  colorCodes: ColorCode | undefined;
}) {
  const connStatus = props.connStatus ?? WSConnStatusShort.Unknown;
  const colorCodes = props.colorCodes ?? defaultColorCodes;
  const colorToken = colorCodes[connStatus] ?? defaultColorCodes[connStatus];
  const label = statusLabels[connStatus];

  return (
    <Tooltip title={label}>
      <Box
        sx={[
          {
            width: 12,
            height: 12,
            borderRadius: "50%",
            backgroundColor: colorToken.light,
            display: "inline-block",
          },
          (theme) =>
            theme.applyStyles("dark", {
              backgroundColor: colorToken.dark,
            }),
        ]}
      />
    </Tooltip>
  );
}

export function convertRTCPeerConnStatus(
  rtcStatus: RTCPeerConnectionState | undefined | null,
): WSConnStatusShort {
  if (rtcStatus === undefined || rtcStatus === null) {
    return WSConnStatusShort.Unknown;
  }
  if (rtcStatus === "connected") {
    return WSConnStatusShort.Online;
  } else if (rtcStatus === "connecting") {
    return WSConnStatusShort.Connecting;
  } else if (
    rtcStatus === "closed" ||
    rtcStatus === "disconnected" ||
    rtcStatus === "failed"
  ) {
    return WSConnStatusShort.Offline;
  } else {
    return WSConnStatusShort.Unknown;
  }
}
