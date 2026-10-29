import { z } from "zod";

export const roleSchema = z.enum(["admin", "operator", "viewer"]);
export type Role = z.infer<typeof roleSchema>;

export const userSchema = z.object({
  id: z.string(),
  name: z.string(),
  email: z.string(),
  role: roleSchema,
  active: z.boolean(),
  created_at: z.string(),
  updated_at: z.string(),
});
export type User = z.infer<typeof userSchema>;

export const sessionSchema = z.object({
  access_token: z.string(),
  token_type: z.string(),
  expires_in: z.number(),
  user: userSchema,
});
export type Session = z.infer<typeof sessionSchema>;

export const categorySchema = z.object({
  id: z.number(),
  name: z.string(),
  code_prefix: z.string(),
});
export type Category = z.infer<typeof categorySchema>;

export const locationKindSchema = z.enum(["storage", "container", "person"]);
export const locationSchema = z.object({
  id: z.number(),
  name: z.string(),
  kind: locationKindSchema,
  parent_id: z.number().nullable(),
});
export type Location = z.infer<typeof locationSchema>;

export const itemKindSchema = z.enum(["asset", "consumable"]);
export type ItemKind = z.infer<typeof itemKindSchema>;
export const itemStatusSchema = z.enum(["functional", "inoperative", "in_repair"]);
export type ItemStatus = z.infer<typeof itemStatusSchema>;
export const availabilitySchema = z.enum(["available", "unavailable"]);
export type Availability = z.infer<typeof availabilitySchema>;

export const custodySchema = z.object({
  assignee_location_id: z.number(),
  assignee_name: z.string(),
  assigned_at: z.string(),
  due_at: z.string().nullable(),
  overdue: z.boolean(),
});
export type Custody = z.infer<typeof custodySchema>;

/** Quantity of a consumable held at one location (only locations with stock are listed). */
export const itemStockSchema = z.object({
  location_id: z.number(),
  location_name: z.string(),
  quantity: z.number(),
});
export type ItemStock = z.infer<typeof itemStockSchema>;

export const itemSchema = z.object({
  id: z.string(),
  code: z.string(),
  description: z.string(),
  category_id: z.number(),
  kind: itemKindSchema,
  status: itemStatusSchema,
  availability: availabilitySchema,
  unit: z.string().nullable(),
  stock_current: z.number(),
  stock_min: z.number(),
  /** Consumables only: stock per location, ordered by location name (empty for assets). */
  stocks: z.array(itemStockSchema),
  low_stock: z.boolean(),
  on_loan: z.boolean(),
  custody: custodySchema.nullish(),
  /** An asset's location; always null for consumables (see `stocks`). */
  location_id: z.number().nullable(),
  last_maintenance_at: z.string().nullable(),
  maintenance_interval_days: z.number().nullable(),
  maintenance_due_at: z.string().nullable(),
  maintenance_overdue: z.boolean(),
  attributes: z.record(z.string(), z.unknown()),
  notes: z.string(),
  created_at: z.string(),
  updated_at: z.string(),
});
export type Item = z.infer<typeof itemSchema>;

export const itemPageSchema = z.object({
  data: z.array(itemSchema),
  total: z.number(),
  limit: z.number(),
  offset: z.number(),
});
export type ItemPage = z.infer<typeof itemPageSchema>;

export const listOf = <T extends z.ZodType>(schema: T) => z.object({ data: z.array(schema) });

export const dashboardSummarySchema = z.object({
  generated_at: z.string(),
  items: z.object({ total: z.number(), assets: z.number(), consumables: z.number() }),
  assets: z.object({
    available: z.number(),
    unavailable: z.number(),
    on_loan: z.number(),
    inoperative: z.number(),
    in_repair: z.number(),
  }),
  low_stock: z.object({
    count: z.number(),
    top: z.array(
      z.object({
        id: z.string(),
        code: z.string(),
        description: z.string(),
        unit: z.string(),
        stock_current: z.number(),
        stock_min: z.number(),
        /** Quantity still to arrive on open purchase orders. */
        pending_ordered: z.number(),
        location_name: z.string().nullable(),
        stocks: z.array(itemStockSchema),
      }),
    ),
  }),
  loans: z.object({
    active: z.number(),
    overdue: z.number(),
    overdue_top: z.array(
      z.object({
        item_id: z.string(),
        code: z.string(),
        description: z.string(),
        assignee_name: z.string(),
        due_at: z.string(),
      }),
    ),
  }),
  maintenance: z.object({
    open_failures: z.number(),
    overdue: z.number(),
    overdue_top: z.array(
      z.object({
        item_id: z.string(),
        code: z.string(),
        description: z.string(),
        maintenance_due_at: z.string(),
      }),
    ),
  }),
});
export type DashboardSummary = z.infer<typeof dashboardSummarySchema>;

export const apiErrorBodySchema = z.object({
  error: z.object({
    code: z.string(),
    message: z.string(),
    fields: z.record(z.string(), z.string()).optional(),
  }),
});

export const pageOf = <T extends z.ZodType>(schema: T) =>
  z.object({ data: z.array(schema), total: z.number(), limit: z.number(), offset: z.number() });

export const movementTypeSchema = z.enum(["opening", "in", "out", "adjust", "transfer"]);
export type MovementType = z.infer<typeof movementTypeSchema>;

export const movementSchema = z.object({
  id: z.number(),
  item_id: z.string(),
  type: movementTypeSchema,
  qty: z.number(),
  stock_before: z.number(),
  stock_after: z.number(),
  /** Consumable opening/in/out/adjust: where it happened. Null for transfers and legacy rows. */
  location_id: z.number().nullable(),
  from_location_id: z.number().nullable(),
  to_location_id: z.number().nullable(),
  /** Quantity moved by a consumable transfer; 0 otherwise. */
  moved_qty: z.number(),
  user_id: z.string().nullable(),
  user_name: z.string().nullable(),
  note: z.string(),
  created_at: z.string(),
});
export type Movement = z.infer<typeof movementSchema>;

export const movementResultSchema = z.object({
  movement: movementSchema,
  stock_current: z.number(),
  location_id: z.number().nullable(),
  stocks: z.array(itemStockSchema),
});
export type MovementResult = z.infer<typeof movementResultSchema>;

export const stockAlertSchema = z.object({
  item_id: z.string(),
  code: z.string(),
  description: z.string(),
  unit: z.string(),
  stock_current: z.number(),
  stock_min: z.number(),
  shortage: z.number(),
  /** Set only when all the stock sits in one location. */
  location_id: z.number().nullable(),
  location_name: z.string().nullable(),
  stocks: z.array(itemStockSchema),
});
export type StockAlert = z.infer<typeof stockAlertSchema>;

export const assignmentSchema = z.object({
  id: z.number(),
  item_id: z.string(),
  item_code: z.string(),
  item_description: z.string(),
  assignee_location_id: z.number(),
  assignee_name: z.string(),
  assignee_kind: z.enum(["person", "container", "storage"]),
  origin_location_id: z.number().nullable(),
  origin_name: z.string().nullable(),
  assigned_at: z.string(),
  due_at: z.string().nullable(),
  overdue: z.boolean(),
  returned_at: z.string().nullable(),
  returned_to_location_id: z.number().nullable(),
  returned_to_name: z.string().nullable(),
  assigned_by: z.string(),
  assigned_by_name: z.string(),
  returned_by: z.string().nullable(),
  returned_by_name: z.string().nullable(),
  assign_note: z.string(),
  return_note: z.string(),
  created_at: z.string(),
});
export type Assignment = z.infer<typeof assignmentSchema>;

export const maintenanceKindSchema = z.enum(["preventive", "corrective"]);
export type MaintenanceKind = z.infer<typeof maintenanceKindSchema>;
export const maintenanceStatusSchema = z.enum(["open", "closed"]);

export const maintenanceRecordSchema = z.object({
  id: z.number(),
  item_id: z.string(),
  item_code: z.string(),
  item_description: z.string(),
  item_status: itemStatusSchema,
  kind: maintenanceKindSchema,
  status: maintenanceStatusSchema,
  reported_at: z.string(),
  description: z.string(),
  destination: z.string().nullable(),
  cost: z.number().nullable(),
  next_due_at: z.string().nullable(),
  closed_at: z.string().nullable(),
  resolution_note: z.string().nullable(),
  reported_by: z.string(),
  reported_by_name: z.string(),
  closed_by: z.string().nullable(),
  closed_by_name: z.string().nullable(),
  created_at: z.string(),
  updated_at: z.string(),
});
export type MaintenanceRecord = z.infer<typeof maintenanceRecordSchema>;

export const dueMaintenanceSchema = z.object({
  item_id: z.string(),
  code: z.string(),
  description: z.string(),
  category_id: z.number(),
  category_name: z.string(),
  status: itemStatusSchema,
  location_id: z.number().nullable(),
  location_name: z.string().nullable(),
  last_maintenance_at: z.string().nullable(),
  maintenance_interval_days: z.number(),
  maintenance_due_at: z.string(),
  maintenance_overdue: z.boolean(),
});
export type DueMaintenance = z.infer<typeof dueMaintenanceSchema>;

export const supplierSchema = z.object({
  id: z.number(),
  name: z.string(),
  contact_name: z.string(),
  email: z.string(),
  phone: z.string(),
  notes: z.string(),
  active: z.boolean(),
  created_at: z.string(),
  updated_at: z.string(),
});
export type Supplier = z.infer<typeof supplierSchema>;

export const purchaseOrderStatusSchema = z.enum(["draft", "ordered", "partially_received", "received", "cancelled"]);
export type PurchaseOrderStatus = z.infer<typeof purchaseOrderStatusSchema>;
export const currencySchema = z.enum(["USD", "VES"]);
export type Currency = z.infer<typeof currencySchema>;

export const purchaseOrderNewItemSchema = z.object({
  description: z.string(),
  category_id: z.number(),
  kind: itemKindSchema,
  unit: z.string().nullish(),
  stock_min: z.number().nullish(),
  attributes: z.record(z.string(), z.unknown()).nullish(),
  notes: z.string().nullish(),
});
export type PurchaseOrderNewItem = z.infer<typeof purchaseOrderNewItemSchema>;

export const purchaseOrderLineSchema = z.object({
  id: z.number(),
  item_id: z.string().nullable(),
  new_item: purchaseOrderNewItemSchema.nullable(),
  description: z.string(),
  quantity: z.number(),
  received_qty: z.number(),
  unit_cost_minor: z.number(),
});
export type PurchaseOrderLine = z.infer<typeof purchaseOrderLineSchema>;

export const purchaseReceiptSchema = z.object({
  id: z.number(),
  order_id: z.number(),
  location_id: z.number(),
  received_by: z.string().nullable(),
  received_at: z.string(),
  note: z.string(),
  client_key: z.string().optional(),
  lines: z.array(
    z.object({
      line_id: z.number(),
      quantity: z.number(),
      items: z.array(z.object({ item_id: z.string(), code: z.string().optional(), quantity: z.number() })),
    }),
  ),
});
export type PurchaseReceipt = z.infer<typeof purchaseReceiptSchema>;

export const purchaseOrderSchema = z.object({
  id: z.number(),
  supplier_id: z.number(),
  supplier_name: z.string(),
  status: purchaseOrderStatusSchema,
  currency: currencySchema,
  exchange_rate: z.number().nullable(),
  expected_at: z.string().nullable(),
  notes: z.string(),
  total_minor: z.number(),
  lines: z.array(purchaseOrderLineSchema),
  receipts: z.array(purchaseReceiptSchema).optional(),
  created_at: z.string(),
  updated_at: z.string(),
  ordered_at: z.string().nullable(),
  cancelled_at: z.string().nullable(),
});
export type PurchaseOrder = z.infer<typeof purchaseOrderSchema>;

export const receiveResultSchema = z.object({ receipt: purchaseReceiptSchema, order: purchaseOrderSchema });
export type ReceiveResult = z.infer<typeof receiveResultSchema>;

export const reorderLineSchema = z.object({
  item_id: z.string(),
  code: z.string(),
  description: z.string(),
  unit: z.string(),
  stock_current: z.number(),
  stock_min: z.number(),
  pending_ordered: z.number(),
  suggested_qty: z.number(),
  last_unit_cost_minor: z.number().nullable(),
  last_currency: currencySchema.nullable(),
});
export type ReorderLine = z.infer<typeof reorderLineSchema>;

export const reorderGroupSchema = z.object({
  supplier_id: z.number().nullable(),
  supplier_name: z.string().nullable(),
  lines: z.array(reorderLineSchema),
});
export type ReorderGroup = z.infer<typeof reorderGroupSchema>;

export const reorderSuggestionsSchema = z.object({ groups: z.array(reorderGroupSchema) });
