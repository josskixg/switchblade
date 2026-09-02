/**
 * Resolved chart colors, per theme, as real hex strings.
 *
 * Recharts writes its props straight into SVG attributes and JS style objects.
 * A CSS custom property is NOT a color there: `var(--color-surface-elevated)`
 * resolves to the bare triplet `30 39 49`, which the browser drops — and an
 * invalid SVG `fill` falls back to black, which is what painted an opaque
 * rectangle over the plot area on hover. So charts are themed through props
 * only, from this file, and index.css carries no `.recharts-*` rules at all.
 *
 * Series are assigned in order and passed explicitly. No chart inherits a
 * color from a global rule.
 */

const LIGHT = {
  mode: 'light',
  grid: '#E8EDF1',
  axis: '#5C6D79',
  axisLine: '#D7DFE5',
  text: '#0E161C',
  muted: '#5C6D79',
  tooltipBg: '#FFFFFF',
  tooltipBorder: '#D7DFE5',
  tooltipText: '#0E161C',
  tooltipShadow: '0 4px 8px -2px rgba(14, 22, 28, 0.08), 0 2px 4px -2px rgba(14, 22, 28, 0.04)',
  cursor: 'rgba(11, 107, 133, 0.08)',
  series: ['#0B6B85', '#B65A08', '#8B2F7A', '#4A6B12', '#4A5A66', '#A33619'],
  success: '#0F7A45',
  warning: '#B65A08',
  danger: '#C2261F',
};

const DARK = {
  mode: 'dark',
  grid: '#212A33',
  axis: '#8494A1',
  axisLine: '#2C3742',
  text: '#E8EDF2',
  muted: '#8494A1',
  tooltipBg: '#1E2731',
  tooltipBorder: '#3E4C59',
  tooltipText: '#E8EDF2',
  tooltipShadow: '0 8px 24px -6px rgba(0, 0, 0, 0.6)',
  cursor: 'rgba(55, 160, 189, 0.12)',
  series: ['#37A0BD', '#F0A548', '#D97BC8', '#A3C34A', '#93A6B4', '#E9865F'],
  success: '#3DBF7A',
  warning: '#F0A548',
  danger: '#F2726A',
};

/** @param {'light'|'dark'} mode */
export function getChartTheme(mode) {
  return mode === 'dark' ? DARK : LIGHT;
}

/** Reads the mode straight off the document. Safe during SSR/tests. */
export function currentMode() {
  if (typeof document === 'undefined') return 'light';
  return document.documentElement.classList.contains('dark') ? 'dark' : 'light';
}

/** Ready-made Recharts <Tooltip /> props for the given theme. */
export function tooltipProps(t) {
  return {
    contentStyle: {
      backgroundColor: t.tooltipBg,
      border: `1px solid ${t.tooltipBorder}`,
      borderRadius: 12,
      boxShadow: t.tooltipShadow,
      color: t.tooltipText,
      fontSize: 12,
      padding: '8px 12px',
    },
    labelStyle: { color: t.muted, fontSize: 11, marginBottom: 4 },
    itemStyle: { color: t.tooltipText, fontSize: 12, padding: 0 },
    cursor: { fill: t.cursor },
  };
}

export default getChartTheme;
