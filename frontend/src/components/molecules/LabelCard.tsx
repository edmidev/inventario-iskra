import { QrCode } from "./QrCode";
import { labelUrl } from "../../features/scan/parseScan";

export interface LabelSubject {
  code: string;
  description: string;
}

/** One printable label: QR on the left, code and description on the right (~50x30 mm in print). */
export function LabelCard({ code, description }: LabelSubject) {
  const url = labelUrl(window.location.origin, code);
  return (
    <div className="label-card flex items-center gap-3 bg-white p-2 text-black">
      <QrCode value={url} label={`Código QR de ${code}`} className="label-qr size-24 shrink-0" />
      <div className="min-w-0">
        <p className="font-mono text-sm leading-tight font-bold break-all">{code}</p>
        <p className="mt-1 line-clamp-3 text-xs leading-snug">{description}</p>
      </div>
    </div>
  );
}
