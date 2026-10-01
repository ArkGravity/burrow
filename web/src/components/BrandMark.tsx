export function BrandMark({ className = "" }: { className?: string }) {
  return (
    <img
      className={`brand-mark ${className}`.trim()}
      src="/logo.svg"
      alt=""
      aria-hidden="true"
      width={36}
      height={36}
      draggable={false}
    />
  );
}
