"use client";

import { ChatMessage } from "@/apis/types";
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
        {message.image && <img src={message.image.url} alt={message.message} />}
        <Box sx={{ padding: 2, whiteSpace: "pre-wrap" }}>{message.message}</Box>
      </Card>
    </Box>
  );
}
