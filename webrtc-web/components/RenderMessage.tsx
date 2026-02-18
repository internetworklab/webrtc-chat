"use client";

import {
  ChatMessage,
  ChatMessageFile,
  ChatMessageFileCategory,
  FileTransferStatusEntry,
} from "@/apis/types";
import { InsertDriveFile } from "@mui/icons-material";
import {
  Box,
  Card,
  Typography,
  CircularProgress,
  CircularProgressProps,
} from "@mui/material";
import { Fragment } from "react/jsx-runtime";
import { RenderAvatar } from "./RenderAvatar";

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

function RenderGenericAttachment(props: {
  file: ChatMessageFile;
  alt: string;
  fileTransferStatus: Record<string, FileTransferStatusEntry>;
}) {
  const { file, fileTransferStatus } = props;
  const alt = props.alt || "attachment";

  if (file.category === ChatMessageFileCategory.Image) {
    if (file.url) {
      return <img style={{ maxHeight: "240px" }} src={file.url} alt={alt} />;
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
      return (
        <video
          autoPlay={false}
          controls
          style={{ maxHeight: "240px" }}
          src={file.url}
        />
      );
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

export function RenderMessage(props: {
  message: ChatMessage;
  onAmend?: (amendedMsg: ChatMessage) => void;
  onDelete?: (deletedMsgId: string) => void;
  fileTransferStatus: Record<string, FileTransferStatusEntry>;
  usernameMap: Record<string, string>;
}) {
  // todo: add message edit feature and delete feature in context menu
  const { message, onAmend, onDelete, fileTransferStatus, usernameMap } = props;
  const username = usernameMap[message.fromNodeId] ?? "";

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
      <RenderAvatar username={username} />
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
        {username && <Box sx={{ paddingLeft: 1 }}>{username}</Box>}
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
