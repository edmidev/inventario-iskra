import { useState } from "react";
import { Button } from "../../../components/atoms/Button";
import { Sheet } from "../../../components/molecules/Sheet";
import { useToast } from "../../../components/molecules/Toast";
import { PurchaseOrderForm, ReceiveForm } from "../../../components/organisms/PurchaseOrderForms";
import { describeError } from "../../../lib/api/errors";
import type { PurchaseOrder } from "../../../lib/api/schemas";
import {
  useCancelPurchaseOrder,
  useCreatePurchaseOrder,
  usePlacePurchaseOrder,
  useReceivePurchaseOrder,
  useUpdatePurchaseOrder,
} from "../api/purchaseOrders";
import { valuesToPayload, type OrderValues } from "../orderForm";

interface SheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function CreatePurchaseOrderSheet({ open, onOpenChange, initialValues }: SheetProps & { initialValues?: OrderValues }) {
  const create = useCreatePurchaseOrder();
  const toast = useToast();
  return (
    <Sheet open={open} onOpenChange={onOpenChange} title="Nueva orden de compra" description="Se guarda como borrador hasta que la marques como pedida.">
      <PurchaseOrderForm
        initialValues={initialValues}
        onCancel={() => onOpenChange(false)}
        onSubmit={async (values) => {
          const saved = await create.mutateAsync(valuesToPayload(values));
          toast.success(`Orden #${saved.id} creada como borrador.`);
          onOpenChange(false);
        }}
      />
    </Sheet>
  );
}

export function EditPurchaseOrderSheet({ order, open, onOpenChange }: SheetProps & { order: PurchaseOrder }) {
  const update = useUpdatePurchaseOrder(order.id);
  const toast = useToast();
  return (
    <Sheet open={open} onOpenChange={onOpenChange} title="Editar orden" description={`Orden #${order.id} · borrador`}>
      <PurchaseOrderForm
        order={order}
        onCancel={() => onOpenChange(false)}
        onSubmit={async (values) => {
          await update.mutateAsync(valuesToPayload(values));
          toast.success(`Orden #${order.id} actualizada.`);
          onOpenChange(false);
        }}
      />
    </Sheet>
  );
}

export function ReceiveSheet({ order, open, onOpenChange }: SheetProps & { order: PurchaseOrder }) {
  const receive = useReceivePurchaseOrder(order.id);
  const toast = useToast();
  return (
    <Sheet open={open} onOpenChange={onOpenChange} title="Recibir mercancía" description={`Orden #${order.id} · ${order.supplier_name}`}>
      <ReceiveForm
        order={order}
        onCancel={() => onOpenChange(false)}
        onSubmit={async (payload) => {
          await receive.mutateAsync(payload);
          toast.success(`Recepción registrada en la orden #${order.id}.`);
          onOpenChange(false);
        }}
      />
    </Sheet>
  );
}

export function CancelOrderSheet({ order, open, onOpenChange }: SheetProps & { order: PurchaseOrder }) {
  const cancel = useCancelPurchaseOrder(order.id);
  const toast = useToast();
  const [error, setError] = useState<string | null>(null);
  return (
    <Sheet
      open={open}
      onOpenChange={(next) => {
        if (!next) setError(null);
        onOpenChange(next);
      }}
      title="Cancelar orden"
      description={`Orden #${order.id} · ${order.supplier_name}`}
    >
      <div className="flex flex-col gap-4">
        <p className="text-sm">La orden quedará cancelada y ya no se podrá recibir ni editar.</p>
        {error && (
          <p role="alert" className="rounded-xl bg-bad-soft px-4 py-3 text-sm font-medium text-bad">
            {error}
          </p>
        )}
        <div className="flex gap-3">
          <Button className="flex-1" onClick={() => onOpenChange(false)}>
            Volver
          </Button>
          <Button
            variant="danger"
            className="flex-1"
            disabled={cancel.isPending}
            onClick={async () => {
              setError(null);
              try {
                await cancel.mutateAsync();
                toast.success(`Orden #${order.id} cancelada.`);
                onOpenChange(false);
              } catch (e) {
                setError(describeError(e));
              }
            }}
          >
            {cancel.isPending ? "Cancelando…" : "Cancelar orden"}
          </Button>
        </div>
      </div>
    </Sheet>
  );
}

/** "Marcar como pedida": no form, so failures surface as a toast. */
export function usePlaceOrderAction(order: PurchaseOrder) {
  const place = usePlacePurchaseOrder(order.id);
  const toast = useToast();
  return {
    pending: place.isPending,
    run: async () => {
      try {
        await place.mutateAsync();
        toast.success(`Orden #${order.id} marcada como pedida.`);
      } catch (e) {
        toast.error(describeError(e));
      }
    },
  };
}
