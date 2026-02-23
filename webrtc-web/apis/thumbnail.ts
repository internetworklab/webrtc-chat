import { ChatMessageThumbnail } from "./types";

export type ThumbnailOptions = {
  qualityFactor?: number;
  maxWidth?: number;
  thumbnailMIME?: string;
};

export const defaultQualityFactor = 0.7;
export const defaultMaxWidth = 150;
const defaultThumbnailMIME = "image/jpeg";

export function createThumbnailFromFile(
  file: File,
  options?: ThumbnailOptions,
): Promise<ChatMessageThumbnail> {
  const {
    qualityFactor = defaultQualityFactor,
    maxWidth = defaultMaxWidth,
    thumbnailMIME = defaultThumbnailMIME,
  } = options ?? {};

  return new Promise<ChatMessageThumbnail>((resolve, reject) => {
    const doCreateThumbnail = (
      mediaElement: HTMLImageElement | HTMLVideoElement | null,
    ) => {
      if (!mediaElement) {
        reject(Error("Media element is null"));
        return;
      }

      const canvas = document.createElement("canvas");
      const ctx = canvas.getContext("2d");
      if (!ctx) {
        reject(
          Error(
            "Failed to get canvas context, canvas is not supported by the browser and please upgrade",
          ),
        );
        return;
      }

      const mediaW =
        mediaElement instanceof HTMLImageElement
          ? mediaElement.width
          : mediaElement.videoWidth;
      const mediaH =
        mediaElement instanceof HTMLImageElement
          ? mediaElement.height
          : mediaElement.videoHeight;

      // 2. Set your desired thumbnail dimensions
      const MAX_WIDTH = maxWidth;
      const scaleSize = MAX_WIDTH / mediaW;
      canvas.width = MAX_WIDTH;
      canvas.height = mediaH * scaleSize;

      // 3. Draw and scale
      ctx.drawImage(mediaElement, 0, 0, canvas.width, canvas.height);

      // 4. Export the result (as a Base64 string or Blob)
      try {
        const thumbnailData = canvas.toDataURL(thumbnailMIME, qualityFactor);

        resolve({
          dataURL: thumbnailData,
          mime: thumbnailMIME,
        });
      } catch (e) {
        reject(new Error("Failed to create thumbnail", { cause: e }));
      } finally {
        // Clean up memory
        URL.revokeObjectURL(mediaElement.src);
      }
    };

    let mediaElement: HTMLImageElement | HTMLVideoElement | null = null;
    if (file.type.startsWith("image/")) {
      mediaElement = new Image();
    } else if (file.type.startsWith("video/")) {
      mediaElement = document.createElement("video");
      mediaElement.muted = true;
      mediaElement.onloadeddata = () => {
        (mediaElement as HTMLVideoElement).currentTime = 1;
      };
      mediaElement.onseeked = () => {
        doCreateThumbnail(mediaElement);
      };
    } else {
      reject(Error("Unsupported file type"));
      return;
    }

    mediaElement.src = URL.createObjectURL(file);

    mediaElement.onload = () => {
      doCreateThumbnail(mediaElement);
    };
  });
}
