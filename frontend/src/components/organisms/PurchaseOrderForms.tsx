import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect, useMemo, useRef, useState } from "react";
import { useFieldArray, useWatch, type Resolver } from "react-hook-form";
import { z } from "zod";
import { Button } from "../atoms/Button";
import { Icon } from "../atoms/Icon";
import { Input, Textarea } from "../atoms/Input";
import { ControlledSelect } from "../molecules/ControlledSelect";
import { FormAlert, FormFooter } from "../molecules/FormFooter";
import { FormField } from "../molecules/FormField";
import { RadioCards } from "../molecules/RadioCards";
import { SearchInput } from "../molecules/SearchInput";
import { useCatalogLookup } from "../../features/catalog/api/catalog";
import { locationPath } from "../../features/catalog/locationPath";
import { defaultFilters } from "../../features/items/filters";
import { useItems } from "../../features/items/api/items";
import type { ReceivePayload } from "../../features/purchasing/api/purchaseOrders";
import { formatMinor, currencyOptions, parseMinorUnits } from "../../features/purchasing/money";
import {
  emptyLine,
  emptyOrder,
  linesTotalMinor,
  MAX_ASSET_UNITS,
  orderFieldToForm,
  orderSchema,
  orderToValues,
  parseQuantity,
  remapFields,
  type LineValues,
  type OrderValues,
} from "../../features/purchasing/orderForm";
import { useActiveSuppliers } from "../../features/suppliers/api/suppliers";
import { CreateSupplierSheet } from "../../features/suppliers/components/SupplierSheets";
import type { Currency, Item, PurchaseOrder, Supplier } from "../../lib/api/schemas";
import { useApiForm } from "../../lib/useApiForm";

const kindOptions = [
  { value: "consumable", label: "Consumible" },
  { value: "asset", label: "Activo" },
];
const sourceOptions = [
  { value: "existing", label: "Consumible existente" },
  { value: "new", label: "Ítem nuevo" },
];

function PickerResults({ q, onPick }: { q: string; onPick: (item: Item) => void }) {
  const query = useItems({ ...defaultFilters, q, kind: "consumable" });
  const items = query.data?.pages[0]?.data ?? [];
  if (query.isPending) return <p className="text-sm text-ink-soft">Buscando…</p>;
  if (query.isError) return <p className="text-sm text-bad">No se pudo buscar. Inténtalo de nuevo.</p>;
  if (items.length === 0) return <p className="text-sm text-ink-soft">Sin resultados. Prueba con otra búsqueda o define un ítem nuevo.</p>;
  return (
    <ul aria-label="Resultados" className="flex flex-col gap-1">
      {items.slice(0, 8).map((item) => (
        <li key={item.id}>
          <Button className="w-full justify-start text-left" aria-label={`Elegir ${item.code}`} onClick={() => onPick(item)}>
            <span className="font-mono text-xs">{item.code}</span>
            <span className="min-w-0 flex-1 truncate font-normal">{item.description}</span>
          </Button>
        </li>
      ))}
    </ul>
  );
}

function ItemPicker({ selectedId, selectedLabel, error, onPick, onClear }: {
  selectedId: string;
  selectedLabel: string;
  error?: string;
  onPick: (item: Item) => void;
  onClear: () => void;
}) {
  const [q, setQ] = useState("");
  if (selectedId) {
    return (
      <div className="flex items-center justify-between gap-3 rounded-xl border border-line px-3 py-2">
        <p className="min-w-0 text-sm font-semibold break-words">{selectedLabel}</p>
        <Button onClick={onClear}>Cambiar</Button>
      </div>
    );
  }
  return (
    <div className="flex flex-col gap-2">
      <SearchInput value={q} onChange={setQ} label="Buscar consumible" placeholder="Código o descripción" />
      {q.trim() && <PickerResults q={q.trim()} onPick={onPick} />}
      {error && (
        <p role="alert" className="text-sm font-medium text-bad">
          {error}
        </p>
      )}
    </div>
  );
}

interface PurchaseOrderFormProps {
  /** Present when editing a draft. */
  order?: PurchaseOrder;
  /** Prefill for a new draft (ignored when editing). */
  initialValues?: OrderValues;
  onSubmit: (values: OrderValues) => Promise<void>;
  onCancel: () => void;
}

export function PurchaseOrderForm({ order, initialValues, onSubmit, onCancel }: PurchaseOrderFormProps) {
  const suppliers = useActiveSuppliers();
  const [creatingSupplier, setCreatingSupplier] = useState(false);
  const [createdSupplier, setCreatedSupplier] = useState<Supplier | null>(null);
  const { categories } = useCatalogLookup();
  const { form, submit, formError } = useApiForm<OrderValues>({
    resolver: zodResolver(orderSchema) as Resolver<OrderValues>,
    defaultValues: order ? orderToValues(order) : (initialValues ?? emptyOrder()),
    fields: [
      "supplier_id",
      "currency",
      "exchange_rate",
      "expected_at",
      "notes",
      "lines.*.item_id",
      "lines.*.description",
      "lines.*.category_id",
      "lines.*.kind",
      "lines.*.unit",
      "lines.*.stock_min",
      "lines.*.quantity",
      "lines.*.unit_cost",
    ],
    codeFields: { supplier_inactive: "supplier_id" },
    onSubmit: async (values) => {
      try {
        await onSubmit(values);
      } catch (error) {
        throw remapFields(error, orderFieldToForm);
      }
    },
  });
  const { register, control, formState, setValue } = form;
  const { errors, isSubmitting } = formState;
  const { fields, append, remove } = useFieldArray({ control, name: "lines" });
  const lines = useWatch({ control, name: "lines" });
  const currency = useWatch({ control, name: "currency" }) as Currency;

  const supplierOptions = useMemo(() => {
    const options = (suppliers.data?.pages.flatMap((p) => p.data) ?? []).map((s) => ({ value: String(s.id), label: s.name }));
    // An inactive supplier already on the draft stays selectable so the draft can still be read.
    if (order && !options.some((o) => o.value === String(order.supplier_id))) {
      options.unshift({ value: String(order.supplier_id), label: `${order.supplier_name} (inactivo)` });
    }
    if (createdSupplier && !options.some((o) => o.value === String(createdSupplier.id))) {
      options.push({ value: String(createdSupplier.id), label: createdSupplier.name });
    }
    return options;
  }, [suppliers.data, order, createdSupplier]);
  // Radix Select clears a value whose option is not mounted yet, so the new supplier
  // becomes an option first and is selected from an effect, one render later.
  useEffect(() => {
    if (createdSupplier) setValue("supplier_id", String(createdSupplier.id), { shouldValidate: true });
  }, [createdSupplier, setValue]);
  const categoryOptions = categories.map((c) => ({ value: String(c.id), label: c.name }));

  return (
    <>
    <form onSubmit={submit} noValidate className="flex flex-col gap-4">
      <ControlledSelect
        control={control}
        name="supplier_id"
        label="Proveedor"
        placeholder="Elige un proveedor"
        options={supplierOptions}
        required
        error={errors.supplier_id?.message}
      />
      {!order && (
        <Button onClick={() => setCreatingSupplier(true)}>
          <Icon name="plus" className="size-4" /> Nuevo proveedor
        </Button>
      )}
      <RadioCards legend="Moneda" registration={register("currency")} options={currencyOptions} error={errors.currency?.message} />
      <FormField label="Tasa de cambio" hint="Opcional. Bolívares por dólar, solo como referencia." error={errors.exchange_rate?.message}>
        <Input inputMode="decimal" placeholder="36.50" {...register("exchange_rate")} />
      </FormField>
      <FormField label="Entrega esperada" error={errors.expected_at?.message}>
        <Input type="date" {...register("expected_at")} />
      </FormField>
      <FormField label="Notas" error={errors.notes?.message}>
        <Textarea rows={2} {...register("notes")} />
      </FormField>

      <div className="flex flex-col gap-3">
        <h3 className="text-sm font-semibold">Líneas</h3>
        {fields.map((field, index) => {
          const line: LineValues = lines?.[index] ?? field;
          const lineErrors = errors.lines?.[index];
          const quantity = parseQuantity(line.quantity);
          const cost = parseMinorUnits(line.unit_cost);
          const isNewAsset = line.source === "new" && line.kind === "asset";
          return (
            <section key={field.id} aria-label={`Línea ${index + 1}`} className="flex flex-col gap-3 rounded-card border border-line p-3">
              <div className="flex items-center justify-between gap-2">
                <p className="font-semibold">Línea {index + 1}</p>
                <Button aria-label={`Quitar línea ${index + 1}`} onClick={() => remove(index)}>
                  <Icon name="x" className="size-4" /> Quitar
                </Button>
              </div>
              <RadioCards legend="Qué se compra" registration={register(`lines.${index}.source`)} options={sourceOptions} />
              {line.source === "existing" ? (
                <ItemPicker
                  selectedId={line.item_id}
                  selectedLabel={line.item_label}
                  error={lineErrors?.item_id?.message}
                  onPick={(item) => {
                    setValue(`lines.${index}.item_id`, item.id, { shouldValidate: true });
                    setValue(`lines.${index}.item_label`, `${item.code} · ${item.description}`);
                  }}
                  onClear={() => {
                    setValue(`lines.${index}.item_id`, "");
                    setValue(`lines.${index}.item_label`, "");
                  }}
                />
              ) : (
                <>
                  <FormField label="Descripción" required error={lineErrors?.description?.message}>
                    <Input autoComplete="off" {...register(`lines.${index}.description`)} />
                  </FormField>
                  <ControlledSelect
                    control={control}
                    name={`lines.${index}.category_id`}
                    label="Categoría"
                    placeholder="Elige una categoría"
                    options={categoryOptions}
                    required
                    error={lineErrors?.category_id?.message}
                  />
                  <RadioCards legend="Tipo" registration={register(`lines.${index}.kind`)} options={kindOptions} error={lineErrors?.kind?.message} />
                  {line.kind === "consumable" && (
                    <>
                      <FormField label="Unidad" required error={lineErrors?.unit?.message}>
                        <Input autoComplete="off" placeholder="caja, litro, pieza…" {...register(`lines.${index}.unit`)} />
                      </FormField>
                      <FormField label="Stock mínimo" hint="Opcional." error={lineErrors?.stock_min?.message}>
                        <Input inputMode="numeric" {...register(`lines.${index}.stock_min`)} />
                      </FormField>
                    </>
                  )}
                </>
              )}
              <FormField label="Cantidad" required error={lineErrors?.quantity?.message}>
                <Input inputMode="numeric" {...register(`lines.${index}.quantity`)} />
              </FormField>
              {isNewAsset && quantity !== null && quantity <= MAX_ASSET_UNITS && (
                <p className="rounded-xl bg-sunken px-3 py-2 text-sm text-ink-soft">
                  {quantity === 1 ? "Se creará 1 ítem con su código." : `Se crearán ${quantity} ítems con códigos distintos.`}
                </p>
              )}
              <FormField label={`Costo unitario (${currency === "USD" ? "US$" : "Bs."})`} required error={lineErrors?.unit_cost?.message}>
                <Input inputMode="decimal" placeholder="0.00" {...register(`lines.${index}.unit_cost`)} />
              </FormField>
              {quantity !== null && cost !== null && (
                <p className="text-right text-sm text-ink-soft">Subtotal {formatMinor(quantity * cost, currency)}</p>
              )}
            </section>
          );
        })}
        <Button onClick={() => append(emptyLine())}>
          <Icon name="plus" className="size-4" /> Añadir línea
        </Button>
        <p className="text-right font-semibold" aria-live="polite">
          Total {formatMinor(linesTotalMinor(lines ?? []), currency)}
        </p>
      </div>

      <FormAlert message={formError} />
      <FormFooter
        onCancel={onCancel}
        submitLabel={order ? "Guardar cambios" : "Crear orden"}
        pendingLabel={order ? "Guardando…" : "Creando…"}
        isSubmitting={isSubmitting}
      />
    </form>
      <CreateSupplierSheet
        open={creatingSupplier}
        onOpenChange={setCreatingSupplier}
        onCreated={setCreatedSupplier}
      />
    </>
  );
}

interface ReceiveValues {
  location_id: string;
  note: string;
  lines: { line_id: number; quantity: string }[];
}

function receiveSchema(pending: number[]) {
  return z
    .object({
      location_id: z.string().min(1, "Elige la ubicación de destino."),
      note: z.string().trim().max(400, "Máximo 400 caracteres."),
      lines: z.array(z.object({ line_id: z.number(), quantity: z.string() })),
    })
    .superRefine((values, ctx) => {
      let any = false;
      let invalid = false;
      values.lines.forEach((line, index) => {
        const text = line.quantity.trim();
        if (text === "" || text === "0") return;
        if (!/^\d+$/.test(text)) {
          invalid = true;
          ctx.addIssue({ code: "custom", path: ["lines", index, "quantity"], message: "Usa un número entero." });
        } else if (Number(text) > pending[index]) {
          invalid = true;
          ctx.addIssue({ code: "custom", path: ["lines", index, "quantity"], message: `Máximo ${pending[index]}: es lo que falta por recibir.` });
        } else {
          any = true;
        }
      });
      if (!any && !invalid) {
        ctx.addIssue({ code: "custom", path: ["lines", 0, "quantity"], message: "Indica cuánto recibiste en al menos una línea." });
      }
    });
}

interface ReceiveFormProps {
  order: PurchaseOrder;
  onSubmit: (payload: ReceivePayload) => Promise<void>;
  onCancel: () => void;
}

export function ReceiveForm({ order, onSubmit, onCancel }: ReceiveFormProps) {
  const open = useMemo(() => order.lines.filter((line) => line.quantity - line.received_qty > 0), [order.lines]);
  const pending = open.map((line) => line.quantity - line.received_qty);
  const schema = useMemo(() => receiveSchema(open.map((line) => line.quantity - line.received_qty)), [open]);
  const { locations } = useCatalogLookup();
  // One idempotency key per attempt: a retry of the same data reuses it, edited data gets a new one.
  const keyRef = useRef(crypto.randomUUID());
  const { form, submit, formError } = useApiForm<ReceiveValues>({
    resolver: zodResolver(schema) as Resolver<ReceiveValues>,
    defaultValues: {
      location_id: "",
      note: "",
      lines: open.map((line) => ({ line_id: line.id, quantity: String(line.quantity - line.received_qty) })),
    },
    fields: ["location_id", "note", "lines.*.quantity"],
    onSubmit: async (values) => {
      const submitted = values.lines.flatMap((line, index) => (Number(line.quantity) > 0 ? [index] : []));
      const note = values.note.trim();
      try {
        await onSubmit({
          location_id: Number(values.location_id),
          ...(note ? { note } : {}),
          client_key: keyRef.current,
          lines: submitted.map((index) => ({ line_id: values.lines[index].line_id, quantity: Number(values.lines[index].quantity) })),
        });
      } catch (error) {
        throw remapFields(error, (field) => field.replace(/^lines\[(\d+)\]/, (_, k: string) => `lines[${submitted[Number(k)] ?? k}]`));
      }
    },
  });
  const { register, control, formState, watch } = form;
  const { errors, isSubmitting } = formState;
  useEffect(() => {
    const subscription = watch(() => {
      keyRef.current = crypto.randomUUID();
    });
    return () => subscription.unsubscribe();
  }, [watch]);

  const locationOptions = locations
    .filter((location) => location.kind !== "person")
    .map((location) => ({ value: String(location.id), label: locationPath(locations, location.id) ?? location.name }));

  return (
    <form onSubmit={submit} noValidate className="flex flex-col gap-4">
      <ControlledSelect
        control={control}
        name="location_id"
        label="Ubicación de destino"
        placeholder="Elige dónde se guarda"
        options={locationOptions}
        required
        error={errors.location_id?.message}
      />
      <div className="flex flex-col gap-3">
        {open.map((line, index) => (
          <FormField key={line.id} label={`Cantidad de ${line.description}`} hint={`Faltan ${pending[index]} de ${line.quantity}.`} error={errors.lines?.[index]?.quantity?.message}>
            <Input inputMode="numeric" {...register(`lines.${index}.quantity`)} />
          </FormField>
        ))}
      </div>
      <FormField label="Nota" hint="Opcional. Por ejemplo, el número de la guía." error={errors.note?.message}>
        <Input autoComplete="off" {...register("note")} />
      </FormField>
      <FormAlert message={formError} />
      <FormFooter onCancel={onCancel} submitLabel="Registrar recepción" pendingLabel="Registrando…" isSubmitting={isSubmitting} />
    </form>
  );
}
