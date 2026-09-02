import { useChartTheme } from '../../theme/ThemeContext';

/**
 * A trend, not a chart: no axes, no fill, no tooltip. Renders nothing at all
 * when there is no data — a flat line at zero is a claim, and an empty
 * sparkline should not make one.
 */
export default function Sparkline({
  data,
  series = 0,
  color,
  width = 96,
  height = 24,
  className = '',
}) {
  const theme = useChartTheme();
  const points = Array.isArray(data) ? data.filter((n) => typeof n === 'number' && Number.isFinite(n)) : [];
  if (points.length < 2) return null;

  const max = Math.max(...points);
  const min = Math.min(...points);
  const span = max - min || 1;
  const stepX = 100 / (points.length - 1);

  const d = points
    .map((v, i) => `${i === 0 ? 'M' : 'L'}${(i * stepX).toFixed(2)},${(100 - ((v - min) / span) * 100).toFixed(2)}`)
    .join(' ');

  return (
    <svg
      viewBox="0 0 100 100"
      preserveAspectRatio="none"
      width={width}
      height={height}
      className={className}
      role="img"
      aria-hidden="true"
      focusable="false"
    >
      <path
        d={d}
        fill="none"
        stroke={color || theme.series[series % theme.series.length]}
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
        vectorEffect="non-scaling-stroke"
      />
    </svg>
  );
}

export { Sparkline };
