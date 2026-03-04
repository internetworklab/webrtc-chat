import { WSServer } from "./types";
import serversJson from "../servers.json";

/**
 * Build a WebSocket URL from the current origin by appending the given path.
 * Uses `wss://` for HTTPS origins and `ws://` for HTTP origins.
 * @param path - The WebSocket path to append (default: "/ws")
 * @returns Full WebSocket URL
 */
export function appendWsPathToCurrentOrigin(path: string = "/ws"): string {
  if (path.startsWith("ws://") || path.startsWith("wss://")) {
    return path;
  }

  if (typeof window === "undefined") {
    return "";
  }

  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  const host = window.location.host;
  const normalizedPath = path.startsWith("/") ? path : `/${path}`;

  return `${protocol}//${host}${normalizedPath}`;
}

function tryPlaceTestServersToTop(wsServers: WSServer[]): WSServer[] {
  if (typeof window === "undefined") {
    return wsServers;
  }

  const hostname = window.location.hostname;
  const localhostHostnames = ["localhost", "127.0.0.1", "[::1]"];

  const isLocalhost = localhostHostnames.includes(hostname);

  if (!isLocalhost) {
    return wsServers;
  }

  // Find the test server and move it to the front
  const testServerIndex = wsServers.findIndex((server) => server.id === "test");
  if (testServerIndex === -1 || testServerIndex === 0) {
    return wsServers;
  }

  const result = [...wsServers];
  const [testServer] = result.splice(testServerIndex, 1);
  result.unshift(testServer);
  return result;
}

function tryPlaceDN42ServersToTop(wsServers: WSServer[]): WSServer[] {
  if (typeof window === "undefined") {
    return wsServers;
  }

  const hostname = window.location.hostname;

  // Check if hostname ends with dn42 or neonetwork/neo suffix
  const isDN42 = hostname.endsWith(".dn42");
  const isNeoNetwork =
    hostname.endsWith(".neonetwork") || hostname.endsWith(".neo");

  if (!isDN42 && !isNeoNetwork) {
    return wsServers;
  }

  // Find the dn42 server and move it to the front
  const dn42ServerIndex = wsServers.findIndex((server) => server.id === "dn42");
  if (dn42ServerIndex === -1 || dn42ServerIndex === 0) {
    return wsServers;
  }

  const result = [...wsServers];
  const [dn42Server] = result.splice(dn42ServerIndex, 1);
  result.unshift(dn42Server);
  return result;
}

function sortSignallingServers(wsServers: WSServer[]): WSServer[] {
  wsServers = tryPlaceTestServersToTop([...wsServers]);
  wsServers = tryPlaceDN42ServersToTop([...wsServers]);
  return wsServers;
}

export function getSignallingServers(): WSServer[] {
  const servers: WSServer[] = serversJson as any;
  return sortSignallingServers(servers);
}
