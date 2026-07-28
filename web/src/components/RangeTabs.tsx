import type { Range } from "../history/types";

interface RangeTabsProps {
  value: Range | null;
  onChange: (range: Range | null) => void;
}

const ranges: Array<{ label: string; value: Range | null }> = [
  { label: "实时", value: null },
  { label: "1天", value: "1d" },
  { label: "7天", value: "7d" },
  { label: "30天", value: "30d" },
];

export function RangeTabs({ value, onChange }: RangeTabsProps) {
  return (
    <div className="range-strip" role="group" aria-label="时间范围">
      {ranges.map((range) => (
        <button
          key={range.label}
          type="button"
          aria-pressed={value === range.value}
          onClick={() => onChange(range.value)}
        >
          {range.label}
        </button>
      ))}
    </div>
  );
}
