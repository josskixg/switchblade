// The old fill was `bg-border/30` — 1.06:1 against the card it sat in, which
// made every loading state in the app an invisible rectangle.
//
// `variant` sets radius only, never width or height. Around thirty call sites
// size themselves with `className="h-11 w-full"`, and a variant that carried
// its own `h-*` would fight them unpredictably — Tailwind's stylesheet order
// decides which height wins, not the order the caller wrote the classes in.
const variants = {
  none: 'rounded-md',
  block: 'rounded-md',
  text: 'rounded-xs',
  circle: 'rounded-full',
};

function Skeleton({ className = '', variant = 'none' }) {
  return (
    <div
      aria-hidden="true"
      className={`
        bg-skeleton relative overflow-hidden
        motion-safe:before:absolute motion-safe:before:inset-0
        motion-safe:before:bg-gradient-to-r
        motion-safe:before:from-transparent motion-safe:before:to-transparent
        motion-safe:before:via-surface motion-safe:dark:before:via-border-strong
        motion-safe:before:bg-[length:40%_100%] motion-safe:before:bg-no-repeat
        motion-safe:before:animate-shimmer motion-safe:before:content-['']
        ${variants[variant] || variants.none} ${className}
      `}
    />
  );
}

/** n stacked text lines, the last one short like real prose. */
export function SkeletonText({ lines = 3, className = '' }) {
  return (
    <div className={`space-y-2 ${className}`}>
      {Array.from({ length: lines }).map((_, i) => (
        <Skeleton key={i} variant="text" className={`h-4 ${i === lines - 1 ? 'w-2/3' : 'w-full'}`} />
      ))}
    </div>
  );
}

export { Skeleton };
export default Skeleton;
