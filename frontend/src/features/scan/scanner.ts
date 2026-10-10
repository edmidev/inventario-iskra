/**
 * Camera QR scanning. Uses the native BarcodeDetector when the browser has it and falls
 * back to the `qr-scanner` library, which is loaded on demand so it never weighs on the
 * app entry chunk.
 */
export interface ScannerHandle {
  stop: () => void;
  /** True when the active camera exposes a torch. */
  hasTorch: boolean;
  /** Toggles the torch and returns its new state. */
  toggleTorch: () => Promise<boolean>;
}

export type CameraFailure = "denied" | "no-camera" | "insecure" | "failed";

export class CameraError extends Error {
  readonly reason: CameraFailure;
  constructor(reason: CameraFailure, message: string) {
    super(message);
    this.name = "CameraError";
    this.reason = reason;
  }
}

/** Maps any getUserMedia / scanner failure to a user-facing category. */
export function classifyCameraError(error: unknown): CameraFailure {
  if (error instanceof CameraError) return error.reason;
  const name = error instanceof DOMException || error instanceof Error ? error.name : "";
  const message = error instanceof Error ? error.message : typeof error === "string" ? error : "";
  if (name === "NotAllowedError" || name === "SecurityError" || /permission|denied/i.test(message)) return "denied";
  if (name === "NotFoundError" || name === "OverconstrainedError" || name === "DevicesNotFoundError" || /no camera/i.test(message)) {
    return "no-camera";
  }
  return "failed";
}

interface DetectedBarcode {
  rawValue: string;
}
interface BarcodeDetectorLike {
  detect: (source: CanvasImageSource) => Promise<DetectedBarcode[]>;
}
type BarcodeDetectorCtor = new (options?: { formats: string[] }) => BarcodeDetectorLike;

function nativeDetector(): BarcodeDetectorCtor | null {
  const ctor = (globalThis as { BarcodeDetector?: BarcodeDetectorCtor }).BarcodeDetector;
  return typeof ctor === "function" ? ctor : null;
}

async function startNative(video: HTMLVideoElement, onCode: (value: string) => void, Detector: BarcodeDetectorCtor): Promise<ScannerHandle> {
  const stream = await navigator.mediaDevices.getUserMedia({ video: { facingMode: { ideal: "environment" } }, audio: false });
  const detector = new Detector({ formats: ["qr_code"] });
  video.srcObject = stream;
  video.setAttribute("playsinline", "true");
  await video.play().catch(() => undefined);

  const track = stream.getVideoTracks()[0];
  const capabilities = (track?.getCapabilities?.() ?? {}) as { torch?: boolean };
  let torchOn = false;
  let stopped = false;
  let busy = false;

  const timer = window.setInterval(() => {
    if (stopped || busy || video.readyState < 2) return;
    busy = true;
    detector
      .detect(video)
      .then((codes) => {
        const value = codes[0]?.rawValue;
        if (value && !stopped) onCode(value);
      })
      .catch(() => undefined)
      .finally(() => {
        busy = false;
      });
  }, 250);

  return {
    hasTorch: Boolean(capabilities.torch),
    async toggleTorch() {
      torchOn = !torchOn;
      await track?.applyConstraints({ advanced: [{ torch: torchOn } as MediaTrackConstraintSet] });
      return torchOn;
    },
    stop() {
      stopped = true;
      window.clearInterval(timer);
      stream.getTracks().forEach((t) => t.stop());
      video.srcObject = null;
    },
  };
}

async function startLibrary(video: HTMLVideoElement, onCode: (value: string) => void): Promise<ScannerHandle> {
  const { default: QrScanner } = await import("qr-scanner");
  if (!(await QrScanner.hasCamera())) throw new CameraError("no-camera", "No camera available");
  const scanner = new QrScanner(video, (result) => onCode(result.data), {
    preferredCamera: "environment",
    returnDetailedScanResult: true,
    highlightScanRegion: false,
    highlightCodeOutline: false,
    maxScansPerSecond: 4,
  });
  await scanner.start();
  const hasTorch = await scanner.hasFlash().catch(() => false);
  return {
    hasTorch,
    async toggleTorch() {
      await scanner.toggleFlash();
      return scanner.isFlashOn();
    },
    stop() {
      scanner.stop();
      scanner.destroy();
    },
  };
}

export async function startScanner(video: HTMLVideoElement, onCode: (value: string) => void): Promise<ScannerHandle> {
  if (!window.isSecureContext) throw new CameraError("insecure", "Camera requires HTTPS");
  if (!navigator.mediaDevices?.getUserMedia) throw new CameraError("no-camera", "No camera API");
  const Detector = nativeDetector();
  return Detector ? startNative(video, onCode, Detector) : startLibrary(video, onCode);
}
