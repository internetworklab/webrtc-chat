"use client";

import { ChatMessage } from "@/apis/types";
import { Box, Card } from "@mui/material";

export function RenderMessage(props: { message: ChatMessage }) {
  const { message } = props;
  return (
    <Card
      sx={{
        padding: 2,
        display: "flex",
        flexDirection: "row",
        gap: 1,
        flexWrap: "wrap",
        justifyContent: "space-between",
        alignItems: "center",

        maxWidth: "100%",
      }}
    >
      <Box sx={{ whiteSpace: "pre-wrap", width: "max-content" }}>
        Message: {message.message}
      </Box>
    </Card>
  );
}
