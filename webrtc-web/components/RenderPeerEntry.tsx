"use client";

import { ChatMessage, ChatMessageFileCategory, ConnEntry } from "@/apis/types";
import { Badge, Box, MenuItem, Typography } from "@mui/material";
import { RenderAvatar } from "./RenderAvatar";

function getMessagePreviewThumbnail(msgCat: ChatMessageFileCategory): string {
  if (msgCat === ChatMessageFileCategory.Image) {
    return "🖼";
  } else if (msgCat === ChatMessageFileCategory.Video) {
    return "🎥";
  } else if (msgCat === ChatMessageFileCategory.Audio) {
    return "🎵";
  } else if (msgCat === ChatMessageFileCategory.Document) {
    return "📄";
  } else {
    return "📎";
  }
}

function getMessagePreview(msg: ChatMessage, maxChars: number = 50): string {
  if (msg.message) {
    return msg.message.length > maxChars
      ? msg.message.slice(0, maxChars) + "..."
      : msg.message;
  }
  if (msg.file) {
    const thumbnail = getMessagePreviewThumbnail(msg.file.category);
    return msg.file.name
      ? `${thumbnail} ${msg.file.name}`
      : `${thumbnail} File`;
  }
  return "";
}

export function RenderPeerEntry(props: {
  conn: ConnEntry;
  preferredColorIdx?: number;
  activeNodeId: string;
  onSelect: () => void;
  latestUnreadMessage?: ChatMessage;
  numUnreads?: number;
}) {
  const {
    conn,
    activeNodeId,
    onSelect,
    latestUnreadMessage,
    numUnreads,
    preferredColorIdx,
  } = props;
  const hasUnreads = numUnreads !== undefined && numUnreads > 0;

  return (
    <MenuItem
      selected={activeNodeId === conn.node_id}
      onClick={() => {
        onSelect();
      }}
      sx={{
        overflow: "hidden",
        display: "flex",
        alignItems: "flex-start",
        gap: 1,
        flexDirection: "column",
      }}
    >
      <Box
        sx={{
          display: "flex",
          alignItems: "center",
          gap: 1,
          width: "100%",
          paddingTop: 0.5,
          paddingBottom: 0.5,
        }}
      >
        <RenderAvatar
          username={conn.entry?.node_name || conn.node_id}
          preferredColorIdx={preferredColorIdx}
        />
        <Box sx={{ flex: 1, minWidth: 0 }}>
          <Typography
            noWrap
            sx={{ fontWeight: hasUnreads ? "bold" : "normal" }}
          >
            {conn.entry?.node_name || conn.node_id}
          </Typography>
          {hasUnreads && latestUnreadMessage && (
            <Typography
              variant="body2"
              color="text.secondary"
              noWrap
              sx={{ fontWeight: hasUnreads ? "medium" : "normal" }}
            >
              {getMessagePreview(latestUnreadMessage)}
            </Typography>
          )}
        </Box>
        {hasUnreads && (
          <Badge
            badgeContent={numUnreads}
            color="primary"
            max={99}
            sx={{
              "& .MuiBadge-badge": {
                position: "relative",
                transform: "none",
              },
            }}
          />
        )}
      </Box>
    </MenuItem>
  );
}
