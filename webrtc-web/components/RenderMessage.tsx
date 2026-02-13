"use client";

import { ChatMessage } from "@/apis/types";
import { InsertDriveFile } from "@mui/icons-material";
import { Box, Card } from "@mui/material";

export function RenderMessage(props: { message: ChatMessage }) {
  const { message } = props;
  let loadingProgress = "";
  if (message.file?.loading) {
    loadingProgress = `(${Math.round(message.file.loading.progress * 100)}%)`;
  }
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
          position: "relative",
        }}
      >
        {message.file?.loading && (
          <Box
            sx={{
              position: "absolute",
              top: 0,
              right: 0,
              width: `${(1 - message.file.loading.progress) * 100}%`,
              height: "100%",
              backgroundColor: "rgba(0, 0, 0, 0.5)",
            }}
          ></Box>
        )}
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
              {message.file.loading && (
                <Box component="span" sx={{ paddingLeft: 0.5 }}>
                  {loadingProgress}
                </Box>
              )}
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
