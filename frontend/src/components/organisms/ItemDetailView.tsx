import { useState, type ReactNode } from "react";
import { Badge } from "../atoms/Badge";
import { Button } from "../atoms/Button";
import { Icon } from "../atoms/Icon";
import { IconButton } from "../atoms/IconButton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "../atoms/Tabs";
import { AvailabilityBadge, LowStockBadge, StatusBadge } from "../molecules/ItemBadges";
import { formatDate, kindLabels } from "../../lib/format";
import type { Item } from "../../lib/api/schemas";

interface ItemDetailViewProps {
  item: Item;
  categoryName: string | null;
  locationName: string | null;
  canWrite: boolean;
  onEdit: () => void;
  onDelete: () => void;
  onCopyCode: () => void;
  /** Container-rendered tab bodies (they fetch their own data). */
  actions: ReactNode;
  historyTab: ReactNode;
  maintenanceTab: ReactNode;
}

function InfoRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-0.5 rounded-xl bg-sunken px-4 py-3">
      <dt className="text-xs font-semibold tracking-wide text-ink-soft uppercase">{label}</dt>
      <dd className="font-medium break-words">{children}</dd>
    </div>
  );
}

function attributeText(value: unknown): string {
  if (typeof value === "string") return value;
  return typeof value === "object" && value !== null ? JSON.stringify(value) : String(value);
}

export function ItemDetailView({
  item,
  categoryName,
  locationName,
  canWrite,
  onEdit,
  onDelete,
  onCopyCode,
  actions,
  historyTab,
  maintenanceTab,
}: ItemDetailViewProps) {
  const [tab, setTab] = useState("info");
  const attributeEntries = Object.entries(item.attributes);

  return (
    <article className="flex flex-col gap-5">
      <header className="flex flex-col gap-3">
        <div className="flex items-center gap-1">
          <h1 className="font-mono text-2xl font-bold text-accent">{item.code}</h1>
          <IconButton icon="copy" label="Copiar código" onClick={onCopyCode} />
        </div>
        <p className="text-lg font-medium">{item.description}</p>
        <div className="flex flex-wrap gap-1.5">
          <Badge>{kindLabels[item.kind]}</Badge>
          <StatusBadge status={item.status} />
          <AvailabilityBadge item={item} />
          {item.kind === "consumable" && item.low_stock && <LowStockBadge />}
          {item.maintenance_overdue && <Badge tone="bad">Mantenimiento vencido</Badge>}
        </div>
        {canWrite && (
          <div className="flex gap-2">
            <Button onClick={onEdit}>
              <Icon name="edit" className="size-4" /> Editar
            </Button>
            <Button variant="secondary" className="text-bad" onClick={onDelete}>
              <Icon name="trash" className="size-4" /> Eliminar
            </Button>
          </div>
        )}
      </header>

      {actions}

      {item.custody && (
        <section
          aria-label="Préstamo vigente"
          className={`rounded-card border p-4 ${item.custody.overdue ? "border-bad/40 bg-bad-soft" : "border-line bg-accent-soft"}`}
        >
          <h2 className="font-semibold">Prestado a {item.custody.assignee_name}</h2>
          <p className="text-sm">
            Desde {formatDate(item.custody.assigned_at)} ·{" "}
            {item.custody.due_at ? `Vence ${formatDate(item.custody.due_at)}` : "Sin fecha de devolución"}
            {item.custody.overdue && <strong className="text-bad"> · Vencido</strong>}
          </p>
        </section>
      )}

      <Tabs value={tab} onValueChange={setTab}>
        <TabsList label="Secciones del material">
          <TabsTrigger value="info">Info</TabsTrigger>
          <TabsTrigger value="history">Historial</TabsTrigger>
          {item.kind === "asset" && <TabsTrigger value="maintenance">Mantenimiento</TabsTrigger>}
        </TabsList>

        <TabsContent value="info" className="pt-4">
          <dl className="grid gap-2 sm:grid-cols-2">
            <InfoRow label="Categoría">{categoryName ?? "—"}</InfoRow>
            <InfoRow label="Ubicación">{locationName ?? "Sin ubicación"}</InfoRow>
            {item.kind === "consumable" ? (
              <>
                <InfoRow label="Existencias">
                  {item.stock_current} {item.unit}
                </InfoRow>
                <InfoRow label="Stock mínimo">
                  {item.stock_min} {item.unit}
                </InfoRow>
              </>
            ) : (
              <>
                <InfoRow label="Mantenimiento cada">
                  {item.maintenance_interval_days ? `${item.maintenance_interval_days} días` : "Sin programar"}
                </InfoRow>
                <InfoRow label="Próximo mantenimiento">
                  {item.maintenance_due_at ? (
                    <span className={item.maintenance_overdue ? "text-bad" : undefined}>
                      {formatDate(item.maintenance_due_at)}
                      {item.maintenance_overdue && " (vencido)"}
                    </span>
                  ) : (
                    "—"
                  )}
                </InfoRow>
                <InfoRow label="Último mantenimiento">{formatDate(item.last_maintenance_at)}</InfoRow>
              </>
            )}
            <InfoRow label="Registrado">{formatDate(item.created_at)}</InfoRow>
          </dl>

          {attributeEntries.length > 0 && (
            <section className="mt-4">
              <h2 className="mb-2 text-sm font-semibold">Atributos</h2>
              <dl className="grid gap-2 sm:grid-cols-2">
                {attributeEntries.map(([key, value]) => (
                  <InfoRow key={key} label={key}>
                    {attributeText(value)}
                  </InfoRow>
                ))}
              </dl>
            </section>
          )}

          {item.notes && (
            <section className="mt-4">
              <h2 className="mb-2 text-sm font-semibold">Notas</h2>
              <p className="rounded-xl bg-sunken px-4 py-3 whitespace-pre-wrap">{item.notes}</p>
            </section>
          )}
        </TabsContent>

        <TabsContent value="history" className="pt-4">
          {historyTab}
        </TabsContent>
        {item.kind === "asset" && (
          <TabsContent value="maintenance" className="pt-4">
            {maintenanceTab}
          </TabsContent>
        )}
      </Tabs>
    </article>
  );
}
