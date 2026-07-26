interface MetricBarProps {
  label: string;
  value: number | null;
}

function metricLevel(value: number): "healthy" | "watch" | "high" {
  if (value >= 85) {
    return "high";
  }
  if (value >= 60) {
    return "watch";
  }
  return "healthy";
}

export function MetricBar({ label, value }: MetricBarProps) {
  if (value === null || !Number.isFinite(value)) {
    return (
      <div className="metric-bar" aria-label={`${label}暂无数据`}>
        <span className="metric-bar-value">—</span>
        <span className="metric-bar-track" aria-hidden="true" />
      </div>
    );
  }

  const safeValue = Math.min(100, Math.max(0, value));
  const level = metricLevel(safeValue);
  return (
    <div className="metric-bar" aria-label={`${label}${safeValue.toFixed(1)}%`}>
      <span className="metric-bar-value">{safeValue.toFixed(1)}%</span>
      <span className="metric-bar-track" aria-hidden="true">
        <span
          className={`metric-bar-fill metric-bar-fill--${level}`}
          style={{ width: `${safeValue}%` }}
        />
      </span>
    </div>
  );
}
