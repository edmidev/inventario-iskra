import { create } from "qrcode";
import { useId, useMemo } from "react";

interface QrCodeProps {
  value: string;
  /** Accessible description of what the code opens. */
  label: string;
  className?: string;
}

/**
 * Renders a QR as inline SVG straight from the module matrix (no innerHTML, no canvas),
 * so it stays crisp when printed. Always black on white: scanners need the contrast.
 */
export function QrCode({ value, label, className }: QrCodeProps) {
  const { size, path } = useMemo(() => {
    const { modules } = create(value, { errorCorrectionLevel: "M" });
    const segments: string[] = [];
    for (let y = 0; y < modules.size; y++) {
      for (let x = 0; x < modules.size; x++) {
        if (modules.get(x, y)) segments.push(`M${x} ${y}h1v1h-1z`);
      }
    }
    return { size: modules.size, path: segments.join("") };
  }, [value]);
  const quiet = 2;
  const titleId = useId();
  return (
    <svg
      aria-labelledby={titleId}
      data-value={value}
      viewBox={`${-quiet} ${-quiet} ${size + quiet * 2} ${size + quiet * 2}`}
      shapeRendering="crispEdges"
      className={className}
    >
      <title id={titleId}>{label}</title>
      <rect x={-quiet} y={-quiet} width={size + quiet * 2} height={size + quiet * 2} fill="#fff" />
      <path d={path} fill="#000" />
    </svg>
  );
}
