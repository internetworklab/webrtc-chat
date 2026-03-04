"use client";

import { Box, Paper } from "@mui/material";
import { useState } from "react";

export function LeftPanel(props: {
  initW?: number;
  children: React.ReactNode;
}) {
  const { initW = 360, children } = props;
  const [width, setWidth] = useState(initW);
  return (
    <Box
      sx={[
        {
          width: `${width}px`,
          height: "100%",
          position: "relative",
          flexShrink: 0,
          borderRight: "1px solid #eee",
        },
        (theme) =>
          theme.applyStyles("dark", {
            borderRight: "1px solid #333",
          }),
      ]}
    >
      <Paper
        sx={{
          height: "100%",
          display: "flex",
          flexDirection: "column",
          overflow: "hidden",
          borderRadius: 0,
        }}
      >
        <Box sx={{ flex: 1, minHeight: 0, overflow: "auto" }}>{children}</Box>
      </Paper>
      <Box
        onMouseDown={(e) => {
          const w0 = width;
          const x0 = e.clientX;
          const handleMouseMove = (e: MouseEvent) => {
            const x = e.clientX;
            const dw = x - x0;
            const w = w0 + dw;
            setWidth(w);
          };
          window.addEventListener("mousemove", handleMouseMove);
          const handleMouseUp = () => {
            window.removeEventListener("mousemove", handleMouseMove);
            window.removeEventListener("mouseup", handleMouseUp);
          };
          window.addEventListener("mouseup", handleMouseUp);
        }}
        sx={{
          position: "absolute",
          top: 0,
          right: 0,
          width: "6px",
          backgroundColor: "#aaa",
          height: "100%",
          cursor: "col-resize",
          opacity: 0,
          transition: "opacity 0.3s ease-in-out",
          "&:hover": {
            opacity: 1,
          },
        }}
      ></Box>
    </Box>
  );
}
