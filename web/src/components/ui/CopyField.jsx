import { useState } from 'react';
import { Check, Copy, Eye, EyeOff } from 'lucide-react';
import { copyText } from './CodeBlock';

/**
 * One-time secret reveal, used at key creation.
 *
 * It never renders a stored key: key values are not retrievable after
 * creation, and a field that appears to show one teaches the wrong thing.
 */
export default function CopyField({
  value,
  label,
  caption = 'This is shown once. Store it somewhere safe before you close this.',
  maskable = false,
  className = '',
}) {
  const [copied, setCopied] = useState(false);
  const [revealed, setRevealed] = useState(!maskable);

  async function onCopy() {
    const ok = await copyText(value);
    if (!ok) return;
    setCopied(true);
    setTimeout(() => setCopied(false), 1600);
  }

  const shown = revealed ? value : '•'.repeat(Math.min(40, String(value || '').length));

  return (
    <div className={className}>
      {label && <p className="text-label text-text mb-1.5">{label}</p>}
      <div className="flex items-stretch gap-2">
        <code className="flex-1 min-w-0 rounded-sm border border-border bg-surface-sunken px-3 py-2 font-mono text-code text-text truncate">
          {shown}
        </code>
        {maskable && (
          <button
            type="button"
            onClick={() => setRevealed((v) => !v)}
            aria-label={revealed ? 'Hide value' : 'Reveal value'}
            className="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-sm border border-border text-muted hover:bg-surface-hover hover:text-text transition-[background-color,border-color,color] duration-120 ease-swift"
          >
            {revealed ? <EyeOff className="w-4 h-4" aria-hidden="true" /> : <Eye className="w-4 h-4" aria-hidden="true" />}
          </button>
        )}
        <button
          type="button"
          onClick={onCopy}
          aria-label={copied ? 'Copied' : 'Copy to clipboard'}
          className="inline-flex h-9 shrink-0 items-center gap-1.5 rounded-sm border border-border-strong px-3 text-label text-text hover:bg-surface-hover transition-[background-color,border-color,color] duration-120 ease-swift"
        >
          {copied ? <Check className="w-4 h-4 text-success" aria-hidden="true" /> : <Copy className="w-4 h-4" aria-hidden="true" />}
          {copied ? 'Copied' : 'Copy'}
        </button>
      </div>
      {caption && <p className="mt-1.5 text-caption text-muted">{caption}</p>}
    </div>
  );
}

export { CopyField };
