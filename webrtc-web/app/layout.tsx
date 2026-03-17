"use client";

import { Geist, Geist_Mono } from "next/font/google";
import "./globals.css";
import {
  CssBaseline,
  createTheme,
  ThemeProvider,
  useColorScheme,
  Box,
  Tooltip,
  IconButton,
} from "@mui/material";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Fragment } from "react";
import LightModeIcon from "@mui/icons-material/LightMode";
import DarkModeIcon from "@mui/icons-material/DarkMode";
import ContrastIcon from "@mui/icons-material/Contrast";
import { firstLetterCap } from "@/utls/strings";
import { Menu } from "@mui/icons-material";
import { ModeSelector } from "@/components/ModeSelector";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

const queryClient = new QueryClient();

const theme = createTheme({
  colorSchemes: {
    light: {
      palette: {
        background: {
          default: "#f5f5f5",
          paper: "#ffffff",
        },
      },
    },
    dark: true,
  },
  components: {
    MuiCard: {
      styleOverrides: {
        root: {
          borderRadius: 16,
        },
      },
    },
    MuiDialog: {
      styleOverrides: {
        paper: {
          borderRadius: 16,
        },
      },
    },
    MuiButton: {
      styleOverrides: {
        root: {
          borderRadius: 8,
        },
      },
    },
    MuiPaper: {
      styleOverrides: {
        root: {
          borderRadius: 8,
        },
      },
    },
  },
});

// With MUI theme (and CSS baseline, of course)
function WithMUITheme(props: { children: React.ReactNode }) {
  return (
    <ThemeProvider theme={theme}>
      <CssBaseline />
      {props.children}
    </ThemeProvider>
  );
}

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <head>
        <title key="mytitle">{"WebRTC Demo"}</title>
        <meta
          key="mydescription"
          name="description"
          content="Web-based Ping & Traceroute Tool"
        />
      </head>
      <body
        className={`${geistSans.variable} ${geistMono.variable} antialiased`}
      >
        <QueryClientProvider client={queryClient}>
          <WithMUITheme>{children}</WithMUITheme>
        </QueryClientProvider>
      </body>
    </html>
  );
}
