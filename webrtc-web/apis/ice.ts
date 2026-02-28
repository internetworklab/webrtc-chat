const defaultInternetICEServers: string[] = ["stun:stun.l.google.com:19302"];
const defaultDN42ICEServers: string[] = [
  "stun:webrtc-stun.duststars.dn42:3478",
];

/**
 * Get comma-separated strings from an environment variable or return defaults.
 * @param envName - The name of the environment variable
 * @param defaultValues - The default values to return if the env is empty/not set
 * @returns Array of strings from the environment variable or defaults
 */
function getCSVStringsFromEnvOrDefault(
  envName: string,
  defaultValues: string[],
): string[] {
  const envValue = process.env[envName];

  if (!envValue || envValue.trim() === "") {
    return defaultValues;
  }

  const values = envValue
    .split(",")
    .map((url) => url.trim())
    .filter((url) => url !== "");

  if (values.length === 0) {
    return defaultValues;
  }

  return values;
}

/**
 * Get ICE server URLs from environment variable or return defaults.
 * The environment variable NEXT_PUBLIC_ICE_SERVERS should be a comma-separated string.
 * @returns Array of ICE server URLs
 */
export function getICEServerURLs(): string[] {
  return getCSVStringsFromEnvOrDefault(
    "NEXT_PUBLIC_ICE_SERVERS",
    defaultInternetICEServers,
  );
}

/**
 * Get DN42 ICE server URLs from environment variable or return defaults.
 * The environment variable NEXT_PUBLIC_DN42_ICE_SERVERS should be a comma-separated string.
 * @returns Array of ICE server URLs for DN42 network
 */
export function getDN42ICEServerURLs(): string[] {
  return getCSVStringsFromEnvOrDefault(
    "NEXT_PUBLIC_DN42_ICE_SERVERS",
    defaultDN42ICEServers,
  );
}
