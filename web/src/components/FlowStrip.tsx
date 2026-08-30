import { TumaBadgerLoading, TumaMarkNav } from "./TumaMark";

export type FlowNodeState = "default" | "active" | "failing" | "retrying" | "holding";

type FlowStripProps = {
  provider?: FlowNodeState;
  tuma?: FlowNodeState;
  destination?: FlowNodeState;
  className?: string;
};

function nodeClass(state: FlowNodeState, extra = "") {
  const mod =
    state === "active"
      ? " tuma-flow__node--active"
      : state === "failing"
        ? " tuma-flow__node--failing"
        : state === "retrying"
          ? " tuma-flow__node--retrying"
          : state === "holding"
            ? " tuma-flow__node--holding"
            : "";
  return `tuma-flow__node${mod}${extra ? ` ${extra}` : ""}`;
}

function arrowClass(from: FlowNodeState, to: FlowNodeState) {
  const lit =
    from === "active" ||
    from === "retrying" ||
    from === "holding" ||
    to === "active" ||
    to === "failing" ||
    to === "retrying" ||
    to === "holding";
  const pulse = from === "retrying" || to === "retrying" || from === "holding";
  if (!lit) return "tuma-flow__arrow";
  return pulse ? "tuma-flow__arrow tuma-flow__arrow--pulse" : "tuma-flow__arrow tuma-flow__arrow--lit";
}

export function FlowStrip({
  provider = "default",
  tuma = "active",
  destination = "default",
  className,
}: FlowStripProps) {
  const markSize = 28;
  const mark =
    tuma === "retrying" ? (
      <TumaBadgerLoading size={markSize} />
    ) : (
      <TumaMarkNav size={markSize} className="tuma-flow__mark" />
    );

  return (
    <div className={`tuma-flow${className ? ` ${className}` : ""}`} role="img" aria-label="Webhook flow">
      <span className={nodeClass(provider)}>Provider</span>
      <span className={arrowClass(provider, tuma)} aria-hidden>→</span>
      <span className={nodeClass(tuma, "tuma-flow__node--mark")} aria-label="Tuma">
        <span className="tuma-flow__mark-loading">{mark}</span>
      </span>
      <span className={arrowClass(tuma, destination)} aria-hidden>→</span>
      <span className={nodeClass(destination)}>Destination</span>
    </div>
  );
}
