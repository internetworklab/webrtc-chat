"use client";

import {
  ChatMessage,
  ChatMessageFile,
  ChatMessageFileCategory,
  ChatMessageSongTrack,
  FileTransferStatusEntry,
  Preference,
} from "@/apis/types";
import {
  InsertDriveFile,
  PlayArrow,
  Pause,
  VolumeUp,
  MusicNote,
} from "@mui/icons-material";
import {
  Box,
  Card,
  Typography,
  CircularProgress,
  CircularProgressProps,
  Dialog,
  DialogTitle,
  DialogContent,
  IconButton,
  Slider,
} from "@mui/material";
import { Fragment } from "react/jsx-runtime";
import { RenderAvatar } from "./RenderAvatar";
import { RefObject, useEffect, useRef, useState } from "react";
import { SP } from "next/dist/shared/lib/utils";

function getFileLoadedRatio(
  file: ChatMessageFile,
  fileTransferStatus: Record<string, FileTransferStatusEntry>,
): number | undefined {
  if (file.dcId) {
    const fileLoadingStatus = fileTransferStatus[file.dcId];
    if (fileLoadingStatus) {
      const fileTotalSize = Math.max(1, file.size ?? 0);
      return Math.min(1, fileLoadingStatus.bytesReceived / fileTotalSize);
    }
  }
}

function RenderFile(props: {
  file: ChatMessageFile;
  fileTransferStatus: Record<string, FileTransferStatusEntry>;
}) {
  const { file, fileTransferStatus } = props;
  const fileLoadingStatus = file.dcId
    ? fileTransferStatus[file.dcId]
    : undefined;
  const loadedRatio: number | undefined = getFileLoadedRatio(
    file,
    fileTransferStatus,
  );

  return (
    <Fragment>
      {loadedRatio !== undefined &&
        loadedRatio !== null &&
        fileLoadingStatus &&
        !fileLoadingStatus?.closed && (
          <Box
            sx={{
              position: "absolute",
              top: 0,
              right: 0,
              width: `${(1 - loadedRatio) * 100}%`,
              height: "100%",
              backgroundColor: "rgba(0, 0, 0, 0.5)",
            }}
          ></Box>
        )}
      <Box sx={{ padding: 2 }}>
        {file.url ? (
          <a href={file.url} download={file.name}>
            <InsertDriveFile />
            <Box component="span" sx={{ paddingLeft: 0.5 }}>
              {file.name}
            </Box>
          </a>
        ) : (
          (file.name ?? "(unknown file)")
        )}{" "}
        {loadedRatio !== undefined &&
          loadedRatio !== null &&
          fileLoadingStatus &&
          !fileLoadingStatus?.closed && (
            <Box component="span" sx={{ paddingLeft: 0.5 }}>
              {`(${Math.round(loadedRatio * 100)}%)`}
            </Box>
          )}
      </Box>
    </Fragment>
  );
}

function CircularProgressWithLabel(
  props: CircularProgressProps & { value: number },
) {
  return (
    <Box sx={{ position: "relative", display: "inline-flex" }}>
      <CircularProgress variant="determinate" {...props} />
      <Box
        sx={{
          top: 0,
          left: 0,
          bottom: 0,
          right: 0,
          position: "absolute",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
        }}
      >
        <Typography
          variant="caption"
          component="div"
        >{`${Math.round(props.value)}%`}</Typography>
      </Box>
    </Box>
  );
}

function ThumbnailWithProgress(props: {
  thumbnailDataURL: string;
  progressPercentage: number;
  alt: string;
}) {
  const { thumbnailDataURL, progressPercentage, alt } = props;
  return (
    <Box>
      <img
        style={{ maxHeight: "240px", filter: "blur(1.5rem)" }}
        src={thumbnailDataURL}
        alt={alt}
      />
      <Box
        sx={{
          position: "absolute",
          top: 0,
          left: 0,
          width: "100%",
          height: "100%",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
        }}
      >
        <CircularProgressWithLabel value={progressPercentage} />
      </Box>
    </Box>
  );
}

function RenderVideo(props: { url: string }) {
  const { url } = props;
  const [showPreview, setShowPreview] = useState(false);
  const videoRef = useRef<HTMLVideoElement>(null);

  const handleMouseEnter = () => {
    const video = videoRef.current;
    if (video) {
      video.play().catch((e) => {
        console.error("failed to play/resume video, because:", e);
      });
    }
  };

  const handleMouseLeave = () => {
    const video = videoRef.current;
    if (video) {
      video.pause();
    }
  };

  return (
    <Fragment>
      <video
        muted={true}
        ref={videoRef}
        autoPlay={false}
        controls={false}
        style={{ maxHeight: "240px", cursor: "pointer" }}
        src={url}
        onMouseEnter={handleMouseEnter}
        onMouseLeave={handleMouseLeave}
        onClick={() => {
          setShowPreview(true);
        }}
      />
      <Dialog open={showPreview} onClose={() => setShowPreview(false)}>
        <DialogContent sx={{ padding: 0 }}>
          <video
            autoPlay={false}
            controls={true}
            style={{ width: "100%", maxHeight: "80vh" }}
            src={url}
          />
        </DialogContent>
      </Dialog>
    </Fragment>
  );
}

function RenderImage(props: { url: string; alt: string }) {
  const { url, alt } = props;
  const [showPreview, setShowPreview] = useState(false);
  return (
    <Fragment>
      <img
        style={{ maxHeight: "240px", cursor: "pointer" }}
        onClick={() => setShowPreview(true)}
        src={url}
        alt={alt}
      />
      <Dialog open={showPreview} onClose={() => setShowPreview(false)}>
        <DialogTitle>Preview</DialogTitle>
        <DialogContent sx={{ padding: 0 }}>
          <img
            style={{ width: "100%", objectFit: "contain" }}
            onClick={() => setShowPreview(true)}
            src={url}
            alt={alt}
          />
        </DialogContent>
      </Dialog>
    </Fragment>
  );
}

function RenderGenericAttachment(props: {
  file: ChatMessageFile;
  alt: string;
  fileTransferStatus: Record<string, FileTransferStatusEntry>;
}) {
  const { file, fileTransferStatus } = props;
  const alt = props.alt || "attachment";

  if (file.category === ChatMessageFileCategory.Image) {
    if (file.url) {
      return <RenderImage url={file.url} alt={alt} />;
    } else if (file.thumbnail?.dataURL) {
      return (
        <ThumbnailWithProgress
          thumbnailDataURL={file.thumbnail.dataURL}
          progressPercentage={Math.round(
            (getFileLoadedRatio(file, fileTransferStatus) ?? 0) * 100,
          )}
          alt={alt}
        />
      );
    } else {
      return (
        <Box
          sx={{ height: "240px", width: "240px", backgroundColor: "#000" }}
        ></Box>
      );
    }
  } else if (file.category === ChatMessageFileCategory.Video) {
    if (file.url) {
      return <RenderVideo url={file.url} />;
    } else if (file.thumbnail?.dataURL) {
      return (
        <ThumbnailWithProgress
          thumbnailDataURL={file.thumbnail.dataURL}
          progressPercentage={Math.round(
            (getFileLoadedRatio(file, fileTransferStatus) ?? 0) * 100,
          )}
          alt={alt}
        />
      );
    } else {
      return (
        <Box
          sx={{ height: "240px", width: "240px", backgroundColor: "#000" }}
        ></Box>
      );
    }
  } else if (file.category === ChatMessageFileCategory.File) {
    return <RenderFile file={file} fileTransferStatus={fileTransferStatus} />;
  } else {
    return <Box>Unknown attachment</Box>;
  }
}

function playSong(
  track: MediaStreamTrack,
  audioContext: AudioContext,
  sourceNodeRef: RefObject<AudioNode | null>,
  gainNodeRef: RefObject<GainNode | null>,
  volume: number,
) {
  if (!gainNodeRef.current) {
    gainNodeRef.current = audioContext.createGain();
    gainNodeRef.current!.gain.value = volume;
  }
  if (!sourceNodeRef.current) {
    const mediaStream = new MediaStream([track]);
    const sourceNode = audioContext.createMediaStreamSource(mediaStream);
    sourceNodeRef.current = sourceNode;

    // // for deubgging purposes only
    // const sineWave = audioContext.createOscillator();
    // sourceNodeRef.current = sineWave;
    // sineWave.start();
  }
  gainNodeRef.current!.gain.value = volume;
  sourceNodeRef.current!.connect(gainNodeRef.current!);
  gainNodeRef.current!.connect(audioContext.destination);
  audioContext.resume();

  // quirks of Chrome, I have to create a Audio Element to make it happy to play
  const audio = new Audio();
  audio.srcObject = new MediaStream([track]);
  audio.muted = true;
  audio.play();
}

function stopSong(
  sourceNodeRef: RefObject<AudioNode | null>,
  gainNodeRef: RefObject<GainNode | null>,
) {
  sourceNodeRef.current?.disconnect?.();
  gainNodeRef.current?.disconnect?.();
}

const defaultVolume = 0.5;

function RenderSongTrack(props: {
  audioContextRef: RefObject<AudioContext | null>;
  songTrackMsgPayload: ChatMessageSongTrack;
}) {
  const { songTrackMsgPayload, audioContextRef } = props;
  const [isPlaying, setIsPlaying] = useState<boolean>(false);
  const [volume, setVolume] = useState<number>(
    songTrackMsgPayload.volume ?? defaultVolume,
  );

  const hasThumbnail = songTrackMsgPayload.thumbnail !== undefined;
  const track = songTrackMsgPayload.track;

  const gainNodeRef = useRef<GainNode | null>(null);
  const sourceNodeRef = useRef<AudioNode | null>(null);

  return (
    <Box
      sx={{
        display: "flex",
        alignItems: "center",
        padding: 2,
        minWidth: 280,
        maxWidth: 400,
      }}
    >
      {/* Album art / Thumbnail - only show if thumbnail exists */}
      {hasThumbnail && (
        <Box
          sx={{
            width: 56,
            height: 56,
            borderRadius: 1,
            overflow: "hidden",
            marginRight: 2,
            flexShrink: 0,
          }}
        >
          <img
            src={songTrackMsgPayload.thumbnail!.dataURL}
            alt={songTrackMsgPayload.label || "Album art"}
            style={{ width: "100%", height: "100%", objectFit: "cover" }}
          />
        </Box>
      )}

      {/* Track info and controls */}
      <Box sx={{ flex: 1, minWidth: 0 }}>
        {/* Track name */}
        <Typography noWrap sx={{ fontWeight: 500, marginBottom: 1 }}>
          {songTrackMsgPayload.label || "Unknown Track"}
        </Typography>

        {/* Progress bar / waveform visualization - only show when playing */}
        {isPlaying && songTrackMsgPayload?.progress !== undefined && (
          <Box sx={{ paddingTop: 1, paddingBottom: 2 }}>
            <Box
              sx={{
                height: 4,
                backgroundColor: "divider",
                borderRadius: 2,
                position: "relative",
                overflow: "hidden",
              }}
            >
              <Box
                sx={{
                  position: "absolute",
                  top: 0,
                  left: 0,
                  height: "100%",
                  width: "60%",
                  backgroundColor: "primary.main",
                  borderRadius: 2,
                  transition: "width 0.3s ease",
                }}
              />
            </Box>
          </Box>
        )}

        {/* Controls row */}
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          {/* Play/Pause button */}
          <IconButton
            size="small"
            disabled={!track || track.muted}
            sx={{
              backgroundColor: "primary.main",
              color: "white",
              "&:hover": {
                backgroundColor: "primary.dark",
              },
              "&.Mui-disabled": {
                backgroundColor: "primary.light",
                color: "white",
                opacity: 0.5,
              },
            }}
            onClick={() => {
              if (isPlaying) {
                stopSong(sourceNodeRef, gainNodeRef);
                setIsPlaying(false);
              } else {
                const audioContext = audioContextRef.current;
                if (audioContext && track) {
                  playSong(
                    track,
                    audioContext,
                    sourceNodeRef,
                    gainNodeRef,
                    volume,
                  );
                }
                setIsPlaying(true);
              }
            }}
          >
            {isPlaying ? (
              <Pause fontSize="small" />
            ) : (
              <PlayArrow fontSize="small" />
            )}
          </IconButton>

          {/* Volume control */}
          <VolumeUp sx={{ fontSize: 18, color: "text.secondary" }} />
          <Slider
            size="small"
            value={volume * 100}
            disabled={!track}
            sx={{
              flex: 1,
              marginLeft: 0.5,
              "& .MuiSlider-thumb": {
                width: 12,
                height: 12,
              },
            }}
          />

          {/* Volume percentage */}
          <Typography
            variant="caption"
            sx={{ color: "text.secondary", minWidth: 35 }}
          >
            {Math.round(volume * 100)}%
          </Typography>
        </Box>
      </Box>
    </Box>
  );
}

export function RenderMessage(props: {
  message: ChatMessage;
  onAmend: (amendedMsg: ChatMessage) => void;
  onDelete: (deletedMsgId: string) => void;
  fileTransferStatus: Record<string, FileTransferStatusEntry>;
  userPreferenceMap: Record<string, Preference>;
  audioContextRef: RefObject<AudioContext | null>;
}) {
  // todo: add message edit feature and delete feature in context menu
  const {
    message,
    onAmend,
    onDelete,
    fileTransferStatus,
    userPreferenceMap,
    audioContextRef,
  } = props;
  const peername = userPreferenceMap[message.fromNodeId]?.name ?? "";
  const peercoloridxprefer =
    userPreferenceMap[message.fromNodeId]?.indexOfPreferColor ?? -1;

  return (
    <Box
      data-message-id={message.messageId}
      sx={{
        display: "flex",
        flexDirection: "row",
        gap: 1,
        maxWidth: "100%",
        width: "max-content",
      }}
    >
      <RenderAvatar
        username={peername}
        preferredColorIdx={peercoloridxprefer}
      />
      <Box
        sx={{
          gap: 1,
          flexWrap: "wrap",
          flex: 1,

          maxWidth: "100%",
          position: "relative",
          display: "flex",
          flexDirection: "column",
          overflow: "hidden",
        }}
      >
        {peername && <Box sx={{ paddingLeft: 1 }}>{peername}</Box>}
        <Card
          sx={{
            gap: 1,
            flexWrap: "wrap",
            justifyContent: "space-between",
            alignItems: "center",
            maxWidth: "100%",
            flex: 1,
            position: "relative",
          }}
        >
          {message.file && (
            <RenderGenericAttachment
              file={message.file}
              alt={message.message || ""}
              fileTransferStatus={fileTransferStatus}
            />
          )}
          {message.songTrack && (
            <RenderSongTrack
              songTrackMsgPayload={message.songTrack}
              audioContextRef={audioContextRef}
            />
          )}
          {message.message && (
            <Box
              sx={{
                padding: 2,
                whiteSpace: "pre-wrap",
                wordWrap: "break-word",
                hyphens: "manual",
                hyphenateCharacter: '"-"',
                maxWidth: "100%",
                width: "max-content",
              }}
            >
              {message.message}
            </Box>
          )}
          {message.richText && (
            <Box sx={{ padding: 2, whiteSpace: "pre-wrap" }}>
              {message.richText.content}
            </Box>
          )}
        </Card>
      </Box>
    </Box>
  );
}
