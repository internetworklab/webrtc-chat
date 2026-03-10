import { DataURL } from "./types";

export type ColorToken = {
  light: string;
  dark: string;
};

export const PRESET_COLORS: ColorToken[] = [
  { light: "#EF9A9A", dark: "#F44336" }, // Red
  { light: "#F48FB1", dark: "#E91E63" }, // Pink
  { light: "#CE93D8", dark: "#9C27B0" }, // Purple
  { light: "#B39DDB", dark: "#673AB7" }, // Deep Purple
  { light: "#9FA8DA", dark: "#3F51B5" }, // Indigo
  { light: "#90CAF9", dark: "#2196F3" }, // Blue
  { light: "#80DEEA", dark: "#00BCD4" }, // Cyan
  { light: "#80CBC4", dark: "#009688" }, // Teal
  { light: "#A5D6A7", dark: "#4CAF50" }, // Green
  { light: "#FFCC80", dark: "#FF9800" }, // Orange
];

export function getColorTokenHashFromUsername(
  username: string,
  N: number,
): number {
  let hash = 0;
  for (let i = 0; i < username.length; i++) {
    const char = username.charCodeAt(i);
    hash = (hash << 5) - hash + char;
    hash = hash & hash; // Convert to 32bit integer
  }
  return Math.abs(hash) % N;
}

export function paintFirstLetterAvatar(username: string): DataURL {
  const colorTokenIdx = getColorTokenHashFromUsername(
    username,
    PRESET_COLORS.length,
  );
  const colorToken = PRESET_COLORS[colorTokenIdx % PRESET_COLORS.length];
  const bgColor = colorToken.dark;
  const fgColor = "#fff";
  const canvasW = 450;
  const canvasH = 450;

  const canvas = document.createElement("canvas");
  canvas.width = canvasW;
  canvas.height = canvasH;

  const ctx = canvas.getContext("2d");
  if (!ctx) {
    throw new Error("Failed to get 2d context");
  }

  // Draw background
  ctx.fillStyle = bgColor;
  ctx.fillRect(0, 0, canvasW, canvasH);

  // Draw the first letter (if username is not empty)
  if (username) {
    const firstLetter = username.charAt(0).toUpperCase();
    ctx.fillStyle = fgColor;
    ctx.font = `bold ${canvasW * 0.6}px sans-serif`;
    ctx.textAlign = "center";
    ctx.textBaseline = "middle";
    ctx.fillText(firstLetter, canvasW / 2, canvasH / 2);
  }

  return canvas.toDataURL() as DataURL;
}
