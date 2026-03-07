import { paintFirstLetterAvatar, PRESET_COLORS } from "./colors";
import { DataURL, IAPKind } from "./types";

export interface IAPOperator {
  getAvatar(username: string): Promise<DataURL>;
}

function getDataURLFromBlob(blob: Blob): Promise<DataURL> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onloadend = () => resolve(reader.result as DataURL);
    reader.onerror = reject;
    reader.readAsDataURL(blob);
  });
}

export function githubIAPOperator(): IAPOperator {
  return {
    async getAvatar(username: string): Promise<DataURL> {
      const searchParams = new URLSearchParams();
      searchParams.set("username", username);
      const response = await fetch(`/api/profile/avatar?${searchParams}`);

      try {
        const blob = await response.blob();
        const dataURL = await getDataURLFromBlob(blob);
        return dataURL;
      } catch (err) {
        console.error("failed to get avatar DataURL, falling back to default");
        return paintFirstLetterAvatar(username);
      }
    },
  };
}

export function kioubitIAPOperator(): IAPOperator {
  return {
    async getAvatar(username: string): Promise<DataURL> {
      return paintFirstLetterAvatar(username);
    },
  };
}

export function getIAPOperator(kind: IAPKind): IAPOperator {
  switch (kind) {
    case IAPKind.Kioubit:
      return kioubitIAPOperator();
    case IAPKind.Github:
    case IAPKind.MockIAP:
      return githubIAPOperator();
    default:
      throw new Error(`Unsupported IAP kind: ${kind}`);
  }
}
