import { useEffect, useRef, useState } from "react";
import { DataURL, Profile, ProfileStatus } from "./types";
import { PSKey, usePersistentStorage } from "./persistent";
import { paintFirstLetterAvatar } from "./colors";

export function getProfile(apiPrefix: string) {
  return fetch(`${apiPrefix}/profile`)
    .then((r) => r.json())
    .then((r) => r as Profile);
}

export function getProfileStatus(apiPrefix: string) {
  return fetch(`${apiPrefix}/profile/status`)
    .then((r) => r.json())
    .then((r) => r as ProfileStatus);
}

const tryParseProfile = (j: string | null | undefined): Profile | undefined => {
  try {
    if (j) {
      return JSON.parse(j);
    }
  } catch (_) {}
};

export function getLoginStatusHintTxt(
  loggedIn: boolean | undefined,
  loggedInAs: Profile | undefined,
): string {
  let hintText = "Waiting for authorization ...";
  if (!!loggedIn) {
    hintText = "You are logged in, fetching profile ...";
    if (!!loggedInAs) {
      hintText = `Logged in as ${loggedInAs.displayName || loggedInAs.username || "(unknown)"}`;
    }
  }
  return hintText;
}

export function useLoginStatusPolling(apiPrefix: string, intervalMs: number) {
  const loggedInSt = usePersistentStorage(PSKey.HasLoggedIn);
  const loggedInAsSt = usePersistentStorage(PSKey.LoggedInAs);
  const loggedIn = loggedInSt.getValue() === "true";
  const setLoggedIn = (t: boolean) => loggedInSt.setValue(String(t));

  const loggedInAs: Profile | undefined = tryParseProfile(
    loggedInAsSt.getValue(),
  );
  const setLoggedInAs = (v: Profile) =>
    loggedInAsSt.setValue(JSON.stringify(v));

  const tickerRef = useRef<ReturnType<typeof setTimeout>>(null);
  const clear = () => {
    if (tickerRef.current !== undefined && tickerRef.current !== null) {
      clearInterval(tickerRef.current);
      tickerRef.current = null;
    }
  };
  const [err, setErr] = useState<Error>();

  useEffect(() => {
    tickerRef.current = setInterval(() => {
      getProfileStatus(apiPrefix)
        .then((status) => {
          if (status.logged_in) {
            clear();
            getProfile(apiPrefix)
              .then((profile) => setLoggedInAs(profile))
              .catch((e) => {
                console.error(
                  "Failed to fetch profile even when user is logged in:",
                  e,
                );
                setErr(e);
              });
          }
          setLoggedIn(status.logged_in);
        })
        .catch((e) => {
          console.error("Failed to fetch user login status:", e);
          setErr(e);
        });
    }, intervalMs);

    return () => {
      clear();
    };
  }, [apiPrefix, intervalMs]);

  let hintText = "Waiting for authorization ...";
  if (!!loggedIn) {
    hintText = "You are logged in, fetching profile ...";
    if (!!loggedInAs) {
      hintText = `Logged in as ${loggedInAs.displayName || loggedInAs.username || "(unknown)"}`;
    }
  }

  return {
    loggedIn,
    loggedInAs,
    err,
    clearLoggedInState: () => {
      localStorage.removeItem(PSKey.HasLoggedIn);
      localStorage.removeItem(PSKey.LoggedInAs);
    },
  };
}

function getDataURLFromBlob(blob: Blob): Promise<DataURL> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onloadend = () => resolve(reader.result as DataURL);
    reader.onerror = reject;
    reader.readAsDataURL(blob);
  });
}

export async function getAvatar(username: string): Promise<DataURL> {
  const searchParams = new URLSearchParams();
  searchParams.set("username", username);
  const response = await fetch(`/api/profile/avatar?${searchParams}`);
  const contentType = response.headers.get("Content-Type");
  const isImageContent =
    typeof contentType === "string" && contentType.startsWith("image/");

  if (response.status >= 300 || response.status < 200 || !isImageContent) {
    return paintFirstLetterAvatar(username);
  }

  try {
    const blob = await response.blob();
    const dataURL = await getDataURLFromBlob(blob);
    return dataURL;
  } catch (err) {
    console.error("failed to get avatar DataURL, falling back to default");
    return paintFirstLetterAvatar(username);
  }
}
