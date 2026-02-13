"use client";

import { ChatMessage } from "@/apis/types";
import { InsertDriveFile } from "@mui/icons-material";
import { Box, Card } from "@mui/material";

export function RenderMessage(props: { message: ChatMessage }) {
  const { message } = props;
  return (
    <Box>
      <Card
        sx={{
          gap: 1,
          flexWrap: "wrap",
          justifyContent: "space-between",
          alignItems: "center",
          maxWidth: "100%",
          width: "max-content",
          flexShrink: 0,
        }}
      >
        {message.image && (
          <img
            style={{ maxHeight: "240px" }}
            src={message.image.url}
            alt={message.message}
          />
        )}
        {message.video && (
          <video
            autoPlay={false}
            controls
            style={{ maxHeight: "240px" }}
            src={message.video.url}
          />
        )}
        {message.file && (
          <Box sx={{ padding: 2 }}>
            <a href={message.file.url} download={message.file.name}>
              <InsertDriveFile />
              <Box component="span" sx={{ paddingLeft: 0.5 }}>
                {message.file.name}
              </Box>
            </a>
          </Box>
        )}
        {message.message && (
          <Box sx={{ padding: 2, whiteSpace: "pre-wrap" }}>
            {message.message}
          </Box>
        )}
      </Card>
    </Box>
  );
}
