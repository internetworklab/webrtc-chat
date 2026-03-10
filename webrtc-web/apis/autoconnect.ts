import { useEffect, useRef } from "react";
import { Preference, Profile, WSServer } from "./types";

// automatically connects to pinned server
export function useAutoconnect(
  pinnedserverObject: WSServer | undefined,
  preference: Preference | undefined,
  connect: (srv: WSServer, pref: Preference | undefined) => void,
  loginAs: Profile | undefined,
) {
  useEffect(() => {
    if (!pinnedserverObject) {
      return;
    }
    const it = setTimeout(() => {
      // connect is idempotent by itself, safe to call multiple times.
      // modification of preference still has to submitted via signalling channel before taking effects.
      connect(pinnedserverObject, loginAs ? undefined : preference);
    });

    return () => clearTimeout(it);
  }, [pinnedserverObject, preference, loginAs]);
}
