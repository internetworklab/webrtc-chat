export function FormatRTT(props: { rtt: number | undefined | null }) {
  const { rtt } = props;
  if (
    rtt === undefined ||
    rtt === null ||
    typeof rtt !== "number" ||
    Number.isNaN(rtt)
  ) {
    return <></>;
  }
  return (
    <>
      {rtt
        .toFixed(2)
        .replace(/\.?0+$/, "")
        .concat("ms")}
    </>
  );
}
