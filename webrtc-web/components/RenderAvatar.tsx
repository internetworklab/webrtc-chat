"use client";

import { Box } from "@mui/material";
import { getPreferredColor } from "./ChangePreference";

export function RenderAvatar(props: {
  username: string;
  url?: string;
  size?: "default" | "small" | "large";
  preferredColorIdx?: number | string;
}) {
  const { username, url, size = "default", preferredColorIdx } = props;
  const firstCap =
    username && username.length > 0 ? username[0].toUpperCase() : "";

  const variants = {
    large: "64px",
    default: "48px",
    small: "32px",
  };

  const fontSizeVariants = {
    large: "2rem",
    default: "1.5rem",
    small: "1rem",
  };

  let bgColorUsedLight: string = "orange";
  let bgColorUsedDark: string = "orange";

  const colorToken = getPreferredColor(preferredColorIdx);
  bgColorUsedLight = colorToken.light;
  bgColorUsedDark = colorToken.dark;

  return (
    <Box
      sx={[
        {
          width: variants[size],
          height: variants[size],
          backgroundColor: bgColorUsedLight,
          borderRadius: "100%",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          fontWeight: "bold",
          fontSize: fontSizeVariants[size],
          flexShrink: 0,
          color: "white",
        },
        (theme) =>
          theme.applyStyles("dark", {
            backgroundColor: bgColorUsedDark,
          }),
      ]}
    >
      {firstCap}
    </Box>
  );
}
