import { useState } from "react";

export interface UsePersistentStorageReturn {
  getValue(): string;
  setValue(value: string): void;
}

export function usePersistentStorage(
  key: PSKey | string,
): UsePersistentStorageReturn {
  const [state, setState] = useState<string | undefined>(undefined);
  return {
    getValue() {
      return localStorage?.getItem(key) ?? state ?? "";
    },
    setValue(value: string) {
      localStorage?.setItem(key, value);
      setState(value);
    },
  };
}

export enum PSKey {
  CurrentServer = "current_server",
  PreferredUsername = "preferred_username",
  HasLoggedIn = "has_logged_in",
  LoggingIn = "logging_in",
}
