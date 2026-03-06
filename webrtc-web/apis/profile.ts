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

