"use client";

import authSVG from "./auth.svg";
import authCSS from "./auth.module.css";
import Image from "next/image";

export function KioubitLogin(props: { onClick: () => void }) {
  const { onClick } = props;

  return (
    <button
      onClick={onClick}
      className={authCSS["kioubit-btn-dark"]}
      part="button"
    >
      <Image
        src={authSVG}
        width="35"
        height="35"
        alt="Kioubit.dn42 logo"
        className={authCSS["kioubit-btn-logo"]}
      />
      Authenticate with Kioubit.dn42
    </button>
  );
}
