import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { Badge } from "../components/atoms/Badge";
import { Button } from "../components/atoms/Button";
import { Icon } from "../components/atoms/Icon";
import { LoadingState, ErrorState } from "../components/molecules/StateMessage";
import { useCatalogLookup } from "../features/catalog/api/catalog";
import { usePermissions } from "../features/auth/session";
import { usePurchaseOrder } from "../features/purchasing/api/purchaseOrders";
import {
  CancelOrderSheet,
  EditPurchaseOrderSheet,
  ReceiveSheet,
  usePlaceOrderAction,
} from "../features/purchasing/components/PurchaseOrderSheets";
import { orderStatusLabels, orderStatusTones } from "../features/purchasing/labels";
import { formatMinor } from "../features/purchasing/money";
import { describeError } from "../lib/api/errors";
import type { PurchaseOrder } from "../lib/api/schemas";
import { formatDate, formatDateTime } from "../lib/format";

function Actions({ order }: { order: PurchaseOrder }) {
  const { canOperate } = usePermissions();
  const place = usePlaceOrderAction(order);
  const [editing, setEditing] = useState(false);
  const [receiving, setReceiving] = useState(false);
  const [cancelling, setCancelling] = useState(false);
  if (!canOperate) return null;
  const canReceive = order.status === "ordered" || order.status === "partially_received";
  const canCancel = order.status === "draft" || order.status === "ordered";
  if (order.status === "received" || order.status === "cancelled") return null;
  return (
    <>
      <div className="flex flex-wrap gap-2">
        {order.status === "draft" && (
          <>
            <Button className="flex-1" onClick={() => setEditing(true)}>
              <Icon name="edit" className="size-4" /> Editar
            </Button>
            <Button variant="primary" className="flex-1" disabled={place.pending} onClick={() => void place.run()}>
              Marcar como pedida
            </Button>
          </>
        )}
        {canReceive && (
          <Button variant="primary" className="flex-1" onClick={() => setReceiving(true)}>
            Recibir mercancía
          </Button>
        )}
        {canCancel && (
          <Button variant="danger" className="flex-1" onClick={() => setCancelling(true)}>
            Cancelar orden
          </Button>
        )}
      </div>
      {editing && <EditPurchaseOrderSheet order={order} open onOpenChange={(open) => !open && setEditing(false)} />}
      {receiving && <ReceiveSheet order={order} open onOpenChange={(open) => !open && setReceiving(false)} />}
      {cancelling && <CancelOrderSheet order={order} open onOpenChange={(open) => !open && setCancelling(false)} />}
    </>
  );
}

function Receipts({ order }: { order: PurchaseOrder }) {
  const { locationName } = useCatalogLookup();
  const receipts = order.receipts ?? [];
  if (receipts.length === 0) return null;
  const descriptions = new Map(order.lines.map((line) => [line.id, line.description]));
  return (
    <section aria-label="Recepciones" className="flex flex-col gap-2">
      <h2 className="text-lg font-semibold">Recepciones</h2>
      <ul className="flex flex-col gap-2">
        {receipts.map((receipt) => (
          <li key={receipt.id} className="flex flex-col gap-1 rounded-card border border-line bg-surface p-4 text-sm">
            <p className="font-semibold">
              {formatDateTime(receipt.received_at)} · {locationName(receipt.location_id) ?? `Ubicación ${receipt.location_id}`}
            </p>
            {receipt.note && <p className="text-ink-soft">{receipt.note}</p>}
            <ul className="flex flex-col gap-0.5">
              {receipt.lines.map((line) => (
                <li key={line.line_id}>
                  {line.quantity} × {descriptions.get(line.line_id) ?? `Línea ${line.line_id}`}
                  {line.items.some((i) => i.code) && (
                    <span className="text-ink-soft"> ({line.items.flatMap((i) => (i.code ? [i.code] : [])).join(", ")})</span>
                  )}
                </li>
              ))}
            </ul>
          </li>
        ))}
      </ul>
    </section>
  );
}

export function PurchaseOrderDetailPage() {
  const { id = "" } = useParams();
  const orderId = /^\d+$/.test(id) ? Number(id) : 0;
  const query = usePurchaseOrder(orderId);
  const order = query.data;

  return (
    <div className="flex flex-col gap-4">
      <Link to="/ordenes-compra" className="inline-flex min-h-11 items-center gap-1 text-sm font-semibold text-accent">
        <Icon name="chevron" className="size-4 rotate-180" /> Órdenes de compra
      </Link>
      {query.isPending && orderId !== 0 && <LoadingState label="Cargando orden" />}
      {(query.isError || orderId === 0) && (
        <ErrorState message={query.error ? describeError(query.error) : "No se encontró la orden."} onRetry={orderId ? () => void query.refetch() : undefined} />
      )}
      {order && (
        <>
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <h1 className="text-2xl font-bold">Orden #{order.id}</h1>
              <p className="break-words text-ink-soft">{order.supplier_name}</p>
            </div>
            <Badge tone={orderStatusTones[order.status]}>{orderStatusLabels[order.status]}</Badge>
          </div>

          <dl className="grid grid-cols-2 gap-3 rounded-card border border-line bg-surface p-4 text-sm">
            <div>
              <dt className="text-ink-soft">Moneda</dt>
              <dd className="font-semibold">{order.currency === "USD" ? "Dólares (US$)" : "Bolívares (Bs.)"}</dd>
            </div>
            <div>
              <dt className="text-ink-soft">Entrega esperada</dt>
              <dd className="font-semibold">{formatDate(order.expected_at)}</dd>
            </div>
            {order.exchange_rate !== null && (
              <div>
                <dt className="text-ink-soft">Tasa de cambio</dt>
                <dd className="font-semibold">{order.exchange_rate} Bs. por US$</dd>
              </div>
            )}
            <div>
              <dt className="text-ink-soft">Total</dt>
              <dd className="font-semibold">{formatMinor(order.total_minor, order.currency)}</dd>
            </div>
            {order.notes && (
              <div className="col-span-2">
                <dt className="text-ink-soft">Notas</dt>
                <dd>{order.notes}</dd>
              </div>
            )}
          </dl>

          <Actions order={order} />

          <section aria-label="Líneas" className="flex flex-col gap-2">
            <h2 className="text-lg font-semibold">Líneas</h2>
            {order.lines.length === 0 ? (
              <p className="text-sm text-ink-soft">La orden todavía no tiene líneas. Edítala para añadirlas.</p>
            ) : (
              <ul className="flex flex-col gap-2">
                {order.lines.map((line) => (
                  <li key={line.id} className="flex flex-col gap-1 rounded-card border border-line bg-surface p-4 text-sm">
                    <p className="font-semibold break-words">{line.description}</p>
                    <p className="text-ink-soft">
                      {line.new_item && !line.item_id && <Badge tone="accent">Ítem nuevo</Badge>}{" "}
                      Recibido {line.received_qty} de {line.quantity} · {formatMinor(line.unit_cost_minor, order.currency)} c/u ·{" "}
                      {formatMinor(line.quantity * line.unit_cost_minor, order.currency)}
                    </p>
                    {line.item_id && (
                      <Link to={`/materiales/${line.item_id}`} className="inline-flex min-h-11 items-center font-semibold text-accent">
                        Ver material
                      </Link>
                    )}
                  </li>
                ))}
              </ul>
            )}
          </section>

          <Receipts order={order} />
        </>
      )}
    </div>
  );
}
