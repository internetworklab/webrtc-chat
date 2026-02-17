"use client";

import {
  ChatMessage,
  ChatMessageFile,
  ChatMessageFileCategory,
  FileTransferStatusEntry,
} from "@/apis/types";
import { InsertDriveFile } from "@mui/icons-material";
import { Box, Card } from "@mui/material";
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
          {message.file &&
          message.file.category === ChatMessageFileCategory.Image &&
          message.file.url ? (
            <img
              style={{ maxHeight: "240px" }}
              src={message.file.url}
              alt={message.message}
            />
          ) : message.file?.thumbnail?.dataURL ? (
            <img
              style={{ maxHeight: "240px", filter: "blur(1.5rem)" }}
              src={message.file?.thumbnail?.dataURL}
              alt={message.message}
            />
          ) : (
            <Box
              sx={{ height: "240px", width: "240px", backgroundColor: "#000" }}
            ></Box>
          )}
          {message.file &&
          message.file.category === ChatMessageFileCategory.Video &&
          message.file.url ? (
            <video
              autoPlay={false}
              controls
              style={{ maxHeight: "240px" }}
              src={message.file.url}
            />
          ) : message.file?.thumbnail?.dataURL ? (
            <img
              style={{ maxHeight: "240px", filter: "blur(1.5rem)" }}
              src={message.file?.thumbnail?.dataURL}
              alt={message.message}
            />
          ) : (
            <Box
              sx={{ height: "240px", width: "240px", backgroundColor: "#000" }}
            ></Box>
          )}
          {message.file &&
            message.file.category === ChatMessageFileCategory.File && (
              <RenderFile
                file={message.file}
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
