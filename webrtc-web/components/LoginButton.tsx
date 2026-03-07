import { PSKey, usePersistentStorage } from "@/apis/persistent";
import { useLoginStatusPolling } from "@/apis/profile";
import { IAPKind, IDProvider } from "@/apis/types";
import { Box, Button } from "@mui/material";
import { ReactNode } from "react";
import { KioubitLogin } from "./web-components-declarative/KioubitLoginBtn";

export function IaPLoginButton(props: {
  iapContext: IDProvider;
  onClick: () => void;
  loading: boolean;
}) {
  const { iapContext, onClick, loading } = props;

  const getDisplayName = () => {
    if (!iapContext) {
      return "Connect";
    }
    const displayName = iapContext.displayName;
    if (typeof displayName === "string") {
      return displayName;
    }
    return displayName.en_US;
  };

  switch (iapContext.kind) {
    case IAPKind.Kioubit:
      return <KioubitLogin onClick={onClick} />;
    case IAPKind.Github:
    case IAPKind.MockIAP:
      return (
        <Button
          variant="contained"
          onClick={onClick}
          loading={loading}
          fullWidth
          startIcon={
            iapContext.loginButtonIconDataURL ? (
              <img
                src={iapContext.loginButtonIconDataURL}
                alt="login icon"
                style={{ width: 24, height: 24 }}
              />
            ) : undefined
          }
        >
          {getDisplayName()}
        </Button>
      );
    default:
      return <Button onClick={onClick}>Login</Button>;
  }
}
