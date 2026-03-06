import { useEffect, useRef, useState } from "react";
import { Profile, ProfileStatus } from "./types";

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

export function useLoginStatusPolling(apiPrefix: string, intervalMs: number) {
  const [loggedIn, setLoggedIn] = useState<boolean | undefined>(undefined);
  const [loggedInAs, setLoggedInAs] = useState<Profile | undefined>(undefined);
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

  let hintText = "Please authorize on the OAuth portal";
  if (!!loggedIn) {
    hintText = "You are logged in, fetching profile ...";
    if (!!loggedInAs) {
      hintText = `Logged in as ${loggedInAs.displayName || loggedInAs.username || "(unknown)"}`;
    }
  }

  return { loggedIn, loggedInAs, hintText, err };
}
