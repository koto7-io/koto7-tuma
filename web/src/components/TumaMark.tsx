/** ≤48px contexts: line badger. Larger mesh assets live in /public/brand/. */
export function TumaMarkNav({ size = 36, className }: { size?: number; className?: string }) {
  return (
    <img
      src="/brand/tuma-mark-nav.svg"
      alt=""
      width={size}
      height={size}
      className={className}
      aria-hidden
    />
  );
}

export function TumaBadgerLoading({ size = 96 }: { size?: number }) {
  return (
    <object
      type="image/svg+xml"
      data="/brand/tuma-badger-loading.svg"
      width={size}
      height={size}
      aria-label="Loading"
    />
  );
}
