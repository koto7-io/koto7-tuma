import { TumaMarkNav } from "./TumaMark";

type TumaLogoProps = {
  size?: number;
  wordmark?: boolean;
  wordmarkText?: string;
  large?: boolean;
  href?: string;
  className?: string;
};

export function TumaLogo({
  size = 36,
  wordmark = true,
  wordmarkText = "tuma",
  large = false,
  href,
  className,
}: TumaLogoProps) {
  const inner = (
    <>
      <TumaMarkNav size={size} className="tuma-logo__mark" />
      {wordmark && (
        <span className={`tuma-logo__wordmark${large ? " tuma-logo__wordmark--lg" : ""}`}>
          {wordmarkText}
        </span>
      )}
    </>
  );

  if (href) {
    return (
      <a href={href} className={`tuma-logo${className ? ` ${className}` : ""}`}>
        {inner}
      </a>
    );
  }

  return <div className={`tuma-logo${className ? ` ${className}` : ""}`}>{inner}</div>;
}

export { TumaMarkNav, TumaBadgerLoading } from "./TumaMark";
