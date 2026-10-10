import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Button } from "../components/atoms/Button";
import { Icon } from "../components/atoms/Icon";
import { Input } from "../components/atoms/Input";
import { FormField } from "../components/molecules/FormField";
import { ApiError, describeError } from "../lib/api/errors";
import { fetchItemByCode } from "../features/scan/api/resolveCode";
import { extractItemCode } from "../features/scan/parseScan";
import { classifyCameraError, startScanner, type CameraFailure, type ScannerHandle } from "../features/scan/scanner";

type CameraState = { status: "starting" } | { status: "scanning" } | { status: "unavailable"; reason: CameraFailure };

const cameraMessages: Record<CameraFailure, string> = {
  denied: "No hay permiso para usar la cámara. Actívalo en los ajustes del navegador o escribe el código.",
  "no-camera": "No se encontró una cámara en este dispositivo. Escribe el código del material.",
  insecure: "La cámara solo funciona en una conexión segura (HTTPS). Escribe el código del material.",
  failed: "No se pudo iniciar la cámara. Escribe el código del material.",
};

export function ScanPage() {
  const navigate = useNavigate();
  const videoRef = useRef<HTMLVideoElement>(null);
  const handleRef = useRef<ScannerHandle | null>(null);
  const busyRef = useRef(false);
  const lastRef = useRef<{ value: string; at: number } | null>(null);
  const [camera, setCamera] = useState<CameraState>({ status: "starting" });
  const [torchOn, setTorchOn] = useState(false);
  const [hasTorch, setHasTorch] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [resolving, setResolving] = useState(false);
  const [manual, setManual] = useState("");

  const resolve = useCallback(
    async (raw: string) => {
      if (busyRef.current) return;
      const code = extractItemCode(raw);
      if (!code) {
        setMessage("Ese valor no parece un código de material.");
        return;
      }
      busyRef.current = true;
      setResolving(true);
      setMessage(null);
      try {
        const item = await fetchItemByCode(code);
        navigate(`/materiales/${item.id}`);
      } catch (error) {
        setMessage(
          error instanceof ApiError && error.status === 404 ? `No se encontró ningún material con el código ${code}.` : describeError(error),
        );
      } finally {
        busyRef.current = false;
        setResolving(false);
      }
    },
    [navigate],
  );

  const resolveRef = useRef(resolve);
  useEffect(() => {
    resolveRef.current = resolve;
  }, [resolve]);

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    let cancelled = false;
    startScanner(video, (value) => {
      // The same QR stays in view for many frames; ignore repeats for a few seconds.
      const now = Date.now();
      const last = lastRef.current;
      if (last && last.value === value && now - last.at < 4000) return;
      lastRef.current = { value, at: now };
      void resolveRef.current(value);
    })
      .then((handle) => {
        if (cancelled) {
          handle.stop();
          return;
        }
        handleRef.current = handle;
        setHasTorch(handle.hasTorch);
        setCamera({ status: "scanning" });
      })
      .catch((error: unknown) => {
        if (!cancelled) setCamera({ status: "unavailable", reason: classifyCameraError(error) });
      });
    return () => {
      cancelled = true;
      handleRef.current?.stop();
      handleRef.current = null;
    };
  }, []);

  const toggleTorch = async () => {
    try {
      setTorchOn((await handleRef.current?.toggleTorch()) ?? false);
    } catch {
      setMessage("No se pudo cambiar la linterna.");
    }
  };

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl font-bold">Escanear</h1>

      <section
        aria-label="Cámara"
        className={`relative w-full overflow-hidden rounded-card bg-black ${camera.status === "unavailable" ? "min-h-32" : "aspect-square max-h-[55dvh]"}`}
      >
        <video ref={videoRef} muted playsInline className={`size-full object-cover ${camera.status === "unavailable" ? "hidden" : ""}`} aria-label="Vista de la cámara" />
        {camera.status === "starting" && (
          <output className="absolute inset-0 grid place-items-center text-sm font-medium text-white">Iniciando cámara…</output>
        )}
        {camera.status === "unavailable" && (
          <p role="alert" className="grid min-h-32 place-items-center bg-sunken p-6 text-center text-sm font-medium text-ink">
            {cameraMessages[camera.reason]}
          </p>
        )}
        {camera.status === "scanning" && (
          <>
            <div aria-hidden="true" className="pointer-events-none absolute inset-8 rounded-2xl border-2 border-white/80" />
            {hasTorch && (
              <Button
                variant="secondary"
                className="absolute right-3 bottom-3"
                aria-pressed={torchOn}
                onClick={() => void toggleTorch()}
              >
                <Icon name="flash" className="size-4" /> {torchOn ? "Apagar linterna" : "Linterna"}
              </Button>
            )}
          </>
        )}
      </section>

      {resolving && (
        <output className="text-sm font-medium text-ink-soft">Buscando material…</output>
      )}
      {message && (
        <p role="alert" className="rounded-xl bg-bad-soft px-4 py-3 text-sm font-medium text-bad">
          {message}
        </p>
      )}

      <form
        aria-label="Código manual"
        className="flex flex-col gap-3 rounded-card border border-line bg-surface p-4"
        onSubmit={(event) => {
          event.preventDefault();
          void resolve(manual);
        }}
      >
        <FormField label="Código del material" hint="Por ejemplo ISK-HER-001">
          <Input
            value={manual}
            onChange={(event) => setManual(event.target.value)}
            autoCapitalize="characters"
            autoComplete="off"
            spellCheck={false}
            className="font-mono"
          />
        </FormField>
        <Button type="submit" variant="primary" disabled={resolving || manual.trim() === ""}>
          Buscar
        </Button>
      </form>
    </div>
  );
}
