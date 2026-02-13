"use client";

import { ChatMessage } from "@/apis/types";
import {
  Box,
  IconButton,
  Input,
  MenuItem,
  Menu,
  Tooltip,
  ListItemIcon,
  ListItemText,
  styled,
} from "@mui/material";
import { Fragment, useState } from "react";
import SendIcon from "@mui/icons-material/Send";
import AttachFileIcon from "@mui/icons-material/AttachFile";
import { MouseEvent } from "react";
import { InsertDriveFile, InsertPhoto } from "@mui/icons-material";

const VisuallyHiddenInput = styled("input")({
  clip: "rect(0 0 0 0)",
  clipPath: "inset(50%)",
  height: 1,
  overflow: "hidden",
  position: "absolute",
  bottom: 0,
  left: 0,
  whiteSpace: "nowrap",
  width: 1,
});

export function MessageComposer(props: {
  onMessage: (message: ChatMessage) => void;
}) {
  const [messageInput, setMessageInput] = useState<string>("");

  const doSend = () => {
    const msgTxt = messageInput;
    if (msgTxt.trim() === "") {
      return;
    }

    const msgObject: ChatMessage = {
      messageId: crypto.randomUUID(),
      fromNodeId: "",
      toNodeId: "",
      message: msgTxt,
      timestamp: Date.now(),
    };

    props.onMessage(msgObject);

    setMessageInput("");
  };

  const [shiftPressed, setShiftPressed] = useState(false);

  const handleKeyUp = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "Shift") {
      setShiftPressed(false);
    }
  };

  const handleEnterKeyPress = (
    event: React.KeyboardEvent<HTMLInputElement>,
  ) => {
    if (event.key === "Shift") {
      setShiftPressed(true);
      return;
    }
    if (event.key === "Enter" && !shiftPressed) {
      event.preventDefault();
      event.stopPropagation();
      doSend();
    }
  };

  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);
  const open = Boolean(anchorEl);
  const handleClick = (event: MouseEvent<HTMLButtonElement>) => {
    setAnchorEl(event.currentTarget);
  };
  const handleClose = () => {
    setAnchorEl(null);
  };
  return (
    <Fragment>
      <Box
        sx={{
          borderTop: "1px solid #999",
          display: "flex",
          flexDirection: "row",
          alignItems: "flex-end",
          paddingTop: 1,
          paddingBottom: 1,
          paddingLeft: 1.5,
          paddingRight: 1,
        }}
      >
        <Tooltip title={"Attachment"}>
          <IconButton onClick={handleClick}>
            <AttachFileIcon sx={{ transform: "rotate(30deg)" }} />
          </IconButton>
        </Tooltip>
        <Input
          sx={{ paddingLeft: 1, paddingBottom: 1 }}
          fullWidth
          multiline
          maxRows={8}
          value={messageInput}
          onChange={(e) => setMessageInput(e.target.value)}
          onKeyDown={handleEnterKeyPress}
          onKeyUp={handleKeyUp}
          disableUnderline
        />
        <Tooltip title="Send">
          <IconButton
            onClick={() => {
              doSend();
            }}
          >
            <SendIcon />
          </IconButton>
        </Tooltip>
      </Box>
      <Menu
        anchorOrigin={{
          vertical: "top",
          horizontal: "right",
        }}
        transformOrigin={{
          vertical: "bottom",
          horizontal: "left",
        }}
        anchorEl={anchorEl}
        open={open}
        onClose={handleClose}
      >
        <MenuItem component="label" onClick={handleClose}>
          <ListItemIcon>
            <InsertDriveFile fontSize="small" />
          </ListItemIcon>
          <ListItemText>File</ListItemText>
          <VisuallyHiddenInput type="file" />
        </MenuItem>
        <MenuItem component="label" onClick={handleClose}>
          <ListItemIcon>
            <InsertPhoto fontSize="small" />
          </ListItemIcon>
          <ListItemText>Photo or Video</ListItemText>
          <VisuallyHiddenInput type="file" />
        </MenuItem>
      </Menu>
    </Fragment>
  );
}
