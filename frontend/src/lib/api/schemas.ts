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
  low_stock: z.boolean(),
  on_loan: z.boolean(),
  custody: custodySchema.nullish(),
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
        location_name: z.string().nullable(),
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
  from_location_id: z.number().nullable(),
  to_location_id: z.number().nullable(),
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
});

export const stockAlertSchema = z.object({
  item_id: z.string(),
  code: z.string(),
  description: z.string(),
  unit: z.string(),
  stock_current: z.number(),
  stock_min: z.number(),
  shortage: z.number(),
  location_id: z.number().nullable(),
  location_name: z.string().nullable(),
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
