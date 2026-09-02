import { useState } from 'react';
import { Check, Copy } from 'lucide-react';

async function copyText(text) {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    // Clipboard API is unavailable over plain HTTP, which is how a self-hosted
    // gateway is usually reached on a LAN.
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.setAttribute('readonly', '');
    ta.style.position = 'fixed';
    ta.style.opacity = '0';
    document.body.appendChild(ta);
    ta.select();
    let ok = false;
    try {
      ok = document.execCommand('copy');
    } catch {
      ok = false;
    }
    document.body.removeChild(ta);
    return ok;
  }
}

export function CopyButton({ value, label = 'Copy', className = '' }) {
  const [copied, setCopied] = useState(false);

  async function onCopy() {
    const ok = await copyText(value);
    if (!ok) return;
    setCopied(true);
    setTimeout(() => setCopied(false), 1600);
  }

  return (
    <button
      type="button"
      onClick={onCopy}
      aria-label={copied ? 'Copied' : label}
      className={`inline-flex h-7 items-center gap-1.5 rounded-sm border border-border px-2 text-caption text-text-secondary bg-surface hover:bg-surface-hover hover:text-text transition-[background-color,border-color,color] duration-120 ease-swift ${className}`}
    >
      {copied ? <Check className="w-3.5 h-3.5 text-success" aria-hidden="true" /> : <Copy className="w-3.5 h-3.5" aria-hidden="true" />}
      {copied ? 'Copied' : label}
    </button>
  );
}

/**
 * `tabs` is [{ id, label, code }] for multi-language samples; pass `code`
 * alone for a single snippet.
 */
export default function CodeBlock({ code, language, tabs, className = '' }) {
  const list = Array.isArray(tabs) && tabs.length > 0 ? tabs : null;
  const [activeId, setActiveId] = useState(list ? list[0].id : null);
  const active = list ? list.find((t) => t.id === activeId) || list[0] : null;
  const shown = active ? active.code : code;

  return (
    <div className={`rounded-lg border border-border-subtle bg-surface-sunken overflow-hidden ${className}`}>
      <div className="flex items-center justify-between gap-2 pl-1 pr-2 py-1 border-b border-border-subtle">
        {list ? (
          <div role="tablist" aria-label="Language" className="flex items-center gap-0.5 overflow-x-auto">
            {list.map((t) => (
              <button
                key={t.id}
                type="button"
                role="tab"
                aria-selected={t.id === active.id}
                onClick={() => setActiveId(t.id)}
                className={`h-7 rounded-sm px-2.5 text-caption whitespace-nowrap transition-[background-color,color] duration-120 ease-swift ${
                  t.id === active.id ? 'bg-surface text-text' : 'text-muted hover:text-text'
                }`}
              >
                {t.label}
              </button>
            ))}
          </div>
        ) : (
          <span className="pl-2 text-micro uppercase text-muted">{language || 'shell'}</span>
        )}
        <CopyButton value={shown} />
      </div>
      <pre className="overflow-x-auto p-4 text-code font-mono text-text-secondary">
        <code>{shown}</code>
      </pre>
    </div>
  );
}

export { CodeBlock, copyText };
