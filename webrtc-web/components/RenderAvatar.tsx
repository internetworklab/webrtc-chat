"use client";

import { Box } from "@mui/material";
import { getPreferredColor } from "./ChangePreference";
import { useQuery } from "@tanstack/react-query";
import { getAvatar } from "@/apis/profile";
import {
  getColorTokenHashFromUsername,
  paintFirstLetterAvatar,
  PRESET_COLORS,
} from "@/apis/colors";

export function RenderAvatar(props: {
  username: string;
  size?: "default" | "small" | "large";
  preferredColorIdx?: number | string;
}) {
  const { username, size = "default" } = props;
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
  const preferredColorIdx =
    props.preferredColorIdx === undefined || props.preferredColorIdx === null
      ? getColorTokenHashFromUsername(username, PRESET_COLORS.length)
      : props.preferredColorIdx;

  const colorToken = getPreferredColor(preferredColorIdx);
  bgColorUsedLight = colorToken.light;
  bgColorUsedDark = colorToken.dark;

  // Use React Query to fetch avatar from IAPOperator
  const { data: avatarUrl } = useQuery({
    queryKey: ["avatar", username],
    queryFn: async () => {
      try {
        const dataUrl = await getAvatar(username);
        return dataUrl;
      } catch (error) {
        console.error("Failed to fetch avatar from IAPOperator:", error);
        return paintFirstLetterAvatar(username || "");
      }
    },
  });

  // If we have a valid avatar URL from IAPOperator, render it as an image
  if (avatarUrl) {
    return (
      <Box
        component="img"
        src={avatarUrl}
        alt={username}
        sx={{
          width: variants[size],
          height: variants[size],
          borderRadius: "100%",
          objectFit: "cover",
          flexShrink: 0,
        }}
      />
    );
  }

  // Fallback to default letter avatar
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
