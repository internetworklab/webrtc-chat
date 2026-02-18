import { useCallback } from "react";

const UNREADS_STORAGE_KEY = "webrtc_unread_message_ids";

export type UseUnreadsHookReturn = {
  addUnreadMessageIds: (unreadMsgIds: string[]) => void;
  updateUnreadMessageIds: (currentlyVisibleMessages: string[]) => void;
  getUnreadMessages: () => string[];
};

export function useUnreads(): UseUnreadsHookReturn {
  const getUnreadMessages = useCallback((): string[] => {
    if (typeof window === "undefined") {
      return [];
    }
    const stored = localStorage.getItem(UNREADS_STORAGE_KEY);
    if (!stored) {
      return [];
    }
    try {
      return JSON.parse(stored) as string[];
    } catch {
      return [];
    }
  }, []);

  const addUnreadMessageIds = useCallback(
    (unreadMsgIds: string[]) => {
      if (typeof window === "undefined") {
        return;
      }
      const existing = getUnreadMessages();
      const newUnreads = [...new Set([...existing, ...unreadMsgIds])];
      localStorage.setItem(UNREADS_STORAGE_KEY, JSON.stringify(newUnreads));
    },
    [getUnreadMessages],
  );

  const updateUnreadMessageIds = useCallback(
    (currentlyVisibleMessages: string[]) => {
      if (typeof window === "undefined") {
        return;
      }
      const existing = getUnreadMessages();
      const visibleSet = new Set(currentlyVisibleMessages);
      const remaining = existing.filter((id) => !visibleSet.has(id));
      localStorage.setItem(UNREADS_STORAGE_KEY, JSON.stringify(remaining));
    },
    [getUnreadMessages],
  );

  return {
    addUnreadMessageIds,
    updateUnreadMessageIds,
    getUnreadMessages,
  };
}
