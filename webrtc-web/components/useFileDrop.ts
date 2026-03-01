"use client";

import { useState, DragEvent } from "react";

export function useFileDrop(onFileList: (fileList: FileList) => void) {
  const [showDropArea, setShowDropArea] = useState(false);
  const onMouseOut = () => setShowDropArea(false);
  const onDrop = (ev: DragEvent<HTMLDivElement>) => {
    ev.preventDefault();
    ev.stopPropagation();

    const fileList = ev.dataTransfer?.files;
    if (fileList) {
      onFileList(fileList);
    }

    setShowDropArea(false);
  };

  const onDragOver = (ev: DragEvent<HTMLDivElement>) => {
    ev.preventDefault();
    ev.stopPropagation();
    setShowDropArea(true);
  };

  return { showDropArea, setShowDropArea, onMouseOut, onDrop, onDragOver };
}
