import { RefObject, useState } from "react";

const UNREADS_STORAGE_KEY = "webrtc_unread_message_ids";

export type UseUnreadsHookReturn = {
  unreads: string[];
  setUnreads: (unreads: string[]) => void;
  addUnreadMessageIds: (unreadMsgIds: string[]) => void;
  updateUnreadMessageIds: (currentlyVisibleMessages: string[]) => void;
  getUnreadMessages: () => Set<string>;
};

function doLoad(nodeId: string): string[] {
  if (!nodeId) {
    return [];
  }

  if (typeof window === "undefined") {
    return [];
  }
  const stored = localStorage.getItem(UNREADS_STORAGE_KEY + ":" + nodeId);
  if (!stored) {
    return [];
  }

  try {
    return stored.split(",") as string[];
  } catch {
    return [];
  }
}

// this hook maintains a globally shared pool of unread message IDs.
// and it serves as the single authority of unread message IDs,
// any message isn't really unread unless it has been queued into here,
// any message isn't really read unless it has been removed from here.
export function useUnreads(nodeIdRef: RefObject<string>): UseUnreadsHookReturn {
  const [unreads, setUnreads] = useState<string[] | undefined>(undefined);

  function doStore(unreadMsgIds: string[] | Set<string>, nodeId: string) {
    if (!nodeId) {
      return;
    }

    const ids = Array.from(unreadMsgIds);
    localStorage.setItem(UNREADS_STORAGE_KEY + ":" + nodeId, ids.join(","));
    setUnreads(ids);
  }

  const getUnreadMessages = (): Set<string> => {
    return new Set(doLoad(nodeIdRef.current));
  };

  const addUnreadMessageIds = (unreadMsgIds: string[]) => {
    if (typeof window === "undefined") {
      return;
    }

    const newUnreads = Array.from(
      new Set([...getUnreadMessages(), ...unreadMsgIds]),
    );
    doStore(newUnreads, nodeIdRef.current);
  };

  const updateUnreadMessageIds = (currentlyVisibleMessages: string[]) => {
    if (typeof window === "undefined") {
      return new Set();
    }
    const existing = getUnreadMessages();
    const visibleSet = new Set(currentlyVisibleMessages);
    const remaining = existing.difference(visibleSet);
    doStore(remaining, nodeIdRef.current);
  };

  return {
    unreads: unreads || doLoad(nodeIdRef.current),
    setUnreads: (unreads) => doStore(unreads, nodeIdRef.current),
    addUnreadMessageIds,
    updateUnreadMessageIds,
    getUnreadMessages,
  };
}
