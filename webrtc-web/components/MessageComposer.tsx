"use client";

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
  Paper,
} from "@mui/material";
import { useState } from "react";
import SendIcon from "@mui/icons-material/Send";
import AttachFileIcon from "@mui/icons-material/AttachFile";
import { MouseEvent } from "react";
import { InsertDriveFile, InsertPhoto } from "@mui/icons-material";
import { useId } from "react";

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
  onText?: (text: string) => void;
  onFile?: (file: FileList) => void;
  onPhoto?: (photo: FileList) => void;
  supportAttachment?: boolean;
  disabled?: boolean;
}) {
  const { onFile, onPhoto, supportAttachment, disabled } = props;
  const [messageInput, setMessageInput] = useState<string>("");

  const doSend = () => {
    const msgTxt = messageInput;
    if (msgTxt.trim() === "") {
      return;
    }

    props.onText?.(msgTxt);
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
    if (!supportAttachment) {
      return;
    }
    setAnchorEl(event.currentTarget);
  };
  const handleClose = () => {
    setAnchorEl(null);
  };

  const fileInputId = useId();
  const photoInputId = useId();

  return (
    <Paper sx={{ borderRadius: 0 }}>
      <Box
        sx={{
          display: "flex",
          flexDirection: "row",
          alignItems: "flex-end",
          paddingTop: 1,
          paddingBottom: 1,
          paddingLeft: 1.5,
          paddingRight: 1,
        }}
      >
        <Tooltip
          title={
            !!supportAttachment ? "Attachment" : "Attachment is not supported"
          }
        >
          <IconButton disabled={!supportAttachment} onClick={handleClick}>
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
          disabled={disabled}
          placeholder={
            disabled ? "Waiting for data channel to be opened" : undefined
          }
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
        <MenuItem
          component="label"
          role={undefined}
          htmlFor={fileInputId}
          onClick={handleClose}
        >
          <ListItemIcon>
            <InsertDriveFile fontSize="small" />
          </ListItemIcon>
          <ListItemText>File</ListItemText>
        </MenuItem>
        <MenuItem
          component="label"
          role={undefined}
          onClick={handleClose}
          htmlFor={photoInputId}
        >
          <ListItemIcon>
            <InsertPhoto fontSize="small" />
          </ListItemIcon>
          <ListItemText>Photo or Video</ListItemText>
        </MenuItem>
      </Menu>
      <VisuallyHiddenInput
        id={fileInputId}
        type="file"
        onChange={(ev) => {
          if (ev.target.files && ev.target.files.length > 0) {
            onFile?.(ev.target.files);
          }
        }}
        multiple
      />
      <VisuallyHiddenInput
        id={photoInputId}
        type="file"
        onChange={(ev) => {
          if (ev.target.files && ev.target.files.length > 0) {
            onPhoto?.(ev.target.files);
          }
        }}
        multiple
      />
    </Paper>
  );
}
