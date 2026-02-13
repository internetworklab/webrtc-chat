"use client";

import { ConnEntry } from "@/apis/types";
import { MenuItem, Typography } from "@mui/material";

export function RenderPeerEntry(props: {
  conn: ConnEntry;
  activeNodeId: string;
  onSelect: () => void;
  rtt?: number;
}) {
  const { conn, activeNodeId, onSelect, rtt } = props;
  return (
    <MenuItem
      selected={activeNodeId === conn.node_id}
      onClick={() => {
        onSelect();
      }}
      sx={{
        overflow: "hidden",
        display: "flex",
        alignItems: "center",
        justifyContent: "space-between",
      }}
    >
      {conn.entry?.node_name || conn.node_id}
      {rtt !== undefined && (
        <Typography
          component="span"
          variant="body2"
          gutterBottom={false}
          marginLeft={1}
          noWrap
        >
          {rtt.toFixed(2).replace(/\.?0+$/, "")}ms
        </Typography>
      )}
    </MenuItem>
  );
}
