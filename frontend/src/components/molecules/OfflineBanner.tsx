import { useOnlineStatus } from "../../lib/useOnlineStatus";

/** Sticky notice shown while the device has no connection; the app shell still loads from the service worker. */
export function OfflineBanner() {
  const online = useOnlineStatus();
  if (online) return null;
  return (
    <output className="sticky top-0 z-50 block bg-warn-soft px-4 py-2 pt-[max(0.5rem,env(safe-area-inset-top))] text-center text-sm font-semibold text-warn">
      Sin conexión. Los datos no se actualizan ni se pueden guardar cambios hasta que vuelva la red.
    </output>
  );
}
