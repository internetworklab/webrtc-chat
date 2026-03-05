import { PSKey, usePersistentStorage } from "@/apis/persistent";
import { IAPKind, IDProvider } from "@/apis/types";
import { Button } from "@mui/material";
import { useEffect } from "react";

export function IaPLoginButton(props: {
  iapContext: IDProvider;

  onLoggedIn: () => void;
}) {
  const { iapContext, onLoggedIn } = props;
  const { getValue: getLoggingIn, setValue: setLoggingIn } =
    usePersistentStorage(PSKey.LoggingIn);
  const isLoggingIn = getLoggingIn();
  useEffect(() => {
    if (isLoggingIn !== "true") {
      return;
    }
    const it = setInterval(() => {
      // todo:
      // check if it is logged in
      // if yes, set loggingIn to false, call onLoggedIn (set logginIn to false would also cancels the ticker)
      // otherwise, return this function call, and this would be called at the next tick
    }, 1500);
    return () => clearInterval(it);
  }, [isLoggingIn]);

  const handleClick = () => {
    // start polling (also the polling state would also survives page reload)
    setLoggingIn("true");

    // navigate the user to the oauth2 authorization portal
    window.open(iapContext.loginUrl);
  };

  const loading = false;
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
    case IAPKind.MockIAP:
      return (
        <Button
          variant="contained"
          onClick={handleClick}
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
      return <Button onClick={handleClick}>Login</Button>;
  }
}
