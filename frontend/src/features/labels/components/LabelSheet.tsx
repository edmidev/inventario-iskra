import { Button } from "../../../components/atoms/Button";
import { Icon } from "../../../components/atoms/Icon";
import { LabelCard, type LabelSubject } from "../../../components/molecules/LabelCard";
import { QrCode } from "../../../components/molecules/QrCode";
import { Sheet } from "../../../components/molecules/Sheet";
import { useToast } from "../../../components/molecules/Toast";
import { PrintArea } from "../../../components/organisms/PrintArea";
import { labelUrl } from "../../scan/parseScan";

interface LabelSheetProps {
  item: LabelSubject;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function LabelSheet({ item, open, onOpenChange }: LabelSheetProps) {
  const toast = useToast();
  const url = labelUrl(window.location.origin, item.code);

  const share = async () => {
    try {
      if (navigator.share) {
        await navigator.share({ title: item.code, text: item.description, url });
      } else {
        await navigator.clipboard.writeText(url);
        toast.success("Enlace copiado.");
      }
    } catch (error) {
      // Dismissing the native share dialog is not an error.
      if (!(error instanceof DOMException && error.name === "AbortError")) toast.error("No se pudo compartir el enlace.");
    }
  };

  return (
    <Sheet open={open} onOpenChange={onOpenChange} title="Etiqueta QR" description="Escanéala con la app o con la cámara del teléfono.">
      <div className="flex flex-col gap-4 pb-2">
        <figure className="mx-auto flex w-full max-w-xs flex-col items-center gap-2 rounded-card border border-line bg-white p-4 text-center text-black">
          <QrCode value={url} label={`Código QR de ${item.code}`} className="size-56" />
          <figcaption>
            <span className="block font-mono text-lg font-bold">{item.code}</span>
            <span className="block text-sm">{item.description}</span>
          </figcaption>
        </figure>
        <div className="flex gap-3">
          <Button variant="primary" className="flex-1" onClick={() => window.print()}>
            <Icon name="print" className="size-4" /> Imprimir
          </Button>
          <Button className="flex-1" onClick={() => void share()}>
            <Icon name="share" className="size-4" /> Compartir
          </Button>
        </div>
      </div>
      {open && (
        <PrintArea>
          <div className="label-grid">
            <LabelCard {...item} />
          </div>
        </PrintArea>
      )}
    </Sheet>
  );
}
