import type { Category, DashboardSummary, Item, Location, Role, User } from "../lib/api/schemas";

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

export const consumable = (overrides: Partial<Item> = {}): Item =>
  item({
    id: "22222222-2222-2222-2222-222222222222",
    code: "ISK-CON-001",
    description: "Guantes de nitrilo",
    category_id: 2,
    kind: "consumable",
    availability: "available",
    unit: "caja",
    stock_current: 2,
    stock_min: 5,
    low_stock: true,
    maintenance_interval_days: null,
    maintenance_due_at: null,
    attributes: {},
    ...overrides,
  });

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
