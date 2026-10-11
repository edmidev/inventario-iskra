import type {
  Assignment,
  Category,
  DashboardSummary,
  DueMaintenance,
  Item,
  ItemStock,
  Location,
  MaintenanceRecord,
  Movement,
  Role,
  StockAlert,
  User,
} from "../lib/api/schemas";

export const user = (role: Role = "operator"): User => ({
  id: "u-1",
  name: "Ana Pérez",
  email: "ana@example.com",
  role,
  active: true,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
});

export const session = (role: Role = "operator") => ({
  access_token: `token-${role}`,
  token_type: "Bearer",
  expires_in: 900,
  user: user(role),
});

export const categories: Category[] = [
  { id: 1, name: "Herramientas", code_prefix: "HER" },
  { id: 2, name: "Consumibles", code_prefix: "CON" },
];

export const locations: Location[] = [
  { id: 10, name: "Bodega", kind: "storage", parent_id: null },
  { id: 11, name: "Estante A", kind: "container", parent_id: 10 },
  { id: 12, name: "Juan Gómez", kind: "person", parent_id: null },
];

export const item = (overrides: Partial<Item> = {}): Item => ({
  id: "11111111-1111-1111-1111-111111111111",
  code: "ISK-HER-001",
  description: "Taladro inalámbrico",
  category_id: 1,
  kind: "asset",
  status: "functional",
  availability: "available",
  unit: null,
  stock_current: 1,
  stock_min: 0,
  stocks: [],
  low_stock: false,
  on_loan: false,
  location_id: 11,
  last_maintenance_at: null,
  maintenance_interval_days: 90,
  maintenance_due_at: "2026-12-01T00:00:00Z",
  maintenance_overdue: false,
  attributes: { marca: "Makita" },
  notes: "",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-02T00:00:00Z",
  ...overrides,
});

export const stock = (location_id: number, quantity: number): ItemStock => ({
  location_id,
  location_name: locations.find((l) => l.id === location_id)?.name ?? `Ubicación ${location_id}`,
  quantity,
});

/**
 * A consumable stocked at Bodega. Overriding `stock_current` without `stocks` keeps them consistent
 * (all of it at Bodega); overriding `stocks` without `stock_current` makes the total follow.
 */
export const consumable = (overrides: Partial<Item> = {}): Item => {
  const stocks = overrides.stocks ?? (overrides.stock_current === 0 ? [] : [stock(10, overrides.stock_current ?? 2)]);
  const total = overrides.stock_current ?? stocks.reduce((sum, row) => sum + row.quantity, 0);
  return item({
    id: "22222222-2222-2222-2222-222222222222",
    code: "ISK-CON-001",
    description: "Guantes de nitrilo",
    category_id: 2,
    kind: "consumable",
    availability: "available",
    unit: "caja",
    stock_min: 5,
    low_stock: true,
    location_id: null,
    maintenance_interval_days: null,
    maintenance_due_at: null,
    attributes: {},
    ...overrides,
    ...{ stocks, stock_current: total },
  });
};

export const onLoanItem = (overrides: Partial<Item> = {}): Item =>
  item({
    id: "33333333-3333-3333-3333-333333333333",
    code: "ISK-HER-002",
    description: "Sierra circular",
    availability: "unavailable",
    on_loan: true,
    custody: {
      assignee_location_id: 12,
      assignee_name: "Juan Gómez",
      assigned_at: "2026-09-01T00:00:00Z",
      due_at: "2026-09-15T00:00:00Z",
      overdue: true,
    },
    ...overrides,
  });

export const summary: DashboardSummary = {
  generated_at: "2026-10-10T12:00:00Z",
  items: { total: 48, assets: 40, consumables: 8 },
  assets: { available: 30, unavailable: 10, on_loan: 4, inoperative: 2, in_repair: 3 },
  low_stock: {
    count: 2,
    top: [
      {
        id: "22222222-2222-2222-2222-222222222222",
        code: "ISK-CON-001",
        description: "Guantes de nitrilo",
        unit: "caja",
        stock_current: 2,
        stock_min: 5,
        location_name: "Bodega",
        stocks: [stock(10, 2)],
      },
    ],
  },
  loans: {
    active: 4,
    overdue: 1,
    overdue_top: [
      {
        item_id: "33333333-3333-3333-3333-333333333333",
        code: "ISK-HER-002",
        description: "Sierra circular",
        assignee_name: "Juan Gómez",
        due_at: "2026-09-15T00:00:00Z",
      },
    ],
  },
  maintenance: {
    open_failures: 1,
    overdue: 1,
    overdue_top: [
      {
        item_id: "11111111-1111-1111-1111-111111111111",
        code: "ISK-HER-001",
        description: "Taladro inalámbrico",
        maintenance_due_at: "2026-09-30T00:00:00Z",
      },
    ],
  },
};

export const movement = (overrides: Partial<Movement> = {}): Movement => ({
  id: 1,
  item_id: "22222222-2222-2222-2222-222222222222",
  type: "in",
  qty: 5,
  stock_before: 2,
  stock_after: 7,
  location_id: 10,
  from_location_id: null,
  to_location_id: null,
  moved_qty: 0,
  user_id: "u-1",
  user_name: "Ana Pérez",
  note: "",
  created_at: "2026-10-01T10:00:00Z",
  ...overrides,
});

export const assignment = (overrides: Partial<Assignment> = {}): Assignment => ({
  id: 1,
  item_id: "33333333-3333-3333-3333-333333333333",
  item_code: "ISK-HER-002",
  item_description: "Sierra circular",
  assignee_location_id: 12,
  assignee_name: "Juan Gómez",
  assignee_kind: "person",
  origin_location_id: 11,
  origin_name: "Estante A",
  assigned_at: "2026-09-01T10:00:00Z",
  due_at: "2026-09-15T00:00:00Z",
  overdue: true,
  returned_at: null,
  returned_to_location_id: null,
  returned_to_name: null,
  assigned_by: "u-1",
  assigned_by_name: "Ana Pérez",
  returned_by: null,
  returned_by_name: null,
  assign_note: "",
  return_note: "",
  created_at: "2026-09-01T10:00:00Z",
  ...overrides,
});

export const maintenanceRecord = (overrides: Partial<MaintenanceRecord> = {}): MaintenanceRecord => ({
  id: 1,
  item_id: "11111111-1111-1111-1111-111111111111",
  item_code: "ISK-HER-001",
  item_description: "Taladro inalámbrico",
  item_status: "inoperative",
  kind: "corrective",
  status: "open",
  reported_at: "2026-10-05T09:00:00Z",
  description: "No enciende",
  destination: null,
  cost: null,
  next_due_at: null,
  closed_at: null,
  resolution_note: null,
  reported_by: "u-1",
  reported_by_name: "Ana Pérez",
  closed_by: null,
  closed_by_name: null,
  created_at: "2026-10-05T09:00:00Z",
  updated_at: "2026-10-05T09:00:00Z",
  ...overrides,
});

export const stockAlert = (overrides: Partial<StockAlert> = {}): StockAlert => ({
  item_id: "22222222-2222-2222-2222-222222222222",
  code: "ISK-CON-001",
  description: "Guantes de nitrilo",
  unit: "caja",
  stock_current: 2,
  stock_min: 5,
  shortage: -3,
  location_id: 10,
  location_name: "Bodega",
  stocks: [stock(10, 2)],
  ...overrides,
});

export const dueMaintenance = (overrides: Partial<DueMaintenance> = {}): DueMaintenance => ({
  item_id: "11111111-1111-1111-1111-111111111111",
  code: "ISK-HER-001",
  description: "Taladro inalámbrico",
  category_id: 1,
  category_name: "Herramientas",
  status: "functional",
  location_id: 11,
  location_name: "Estante A",
  last_maintenance_at: "2026-06-01T00:00:00Z",
  maintenance_interval_days: 90,
  maintenance_due_at: "2026-09-30T00:00:00Z",
  maintenance_overdue: true,
  ...overrides,
});

/** Wraps rows in the paginated envelope the list endpoints return. */
export const page = <T,>(data: T[], extra: { total?: number; limit?: number; offset?: number } = {}) => ({
  data,
  total: extra.total ?? data.length,
  limit: extra.limit ?? 20,
  offset: extra.offset ?? 0,
});
