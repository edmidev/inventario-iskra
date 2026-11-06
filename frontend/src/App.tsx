import { lazy, Suspense } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { OfflineBanner } from "./components/molecules/OfflineBanner";
import { LoadingState } from "./components/molecules/StateMessage";
import { AppShell } from "./components/organisms/AppShell";
import { RequireAuth } from "./features/auth/RequireAuth";
import { RequireRole } from "./features/auth/RequireRole";

// Route-level code splitting: each page (with the forms it opens) is its own chunk.
const LoginPage = lazy(() => import("./pages/LoginPage").then((m) => ({ default: m.LoginPage })));
const DashboardPage = lazy(() => import("./pages/DashboardPage").then((m) => ({ default: m.DashboardPage })));
const ItemsPage = lazy(() => import("./pages/ItemsPage").then((m) => ({ default: m.ItemsPage })));
const ItemDetailPage = lazy(() => import("./pages/ItemDetailPage").then((m) => ({ default: m.ItemDetailPage })));
const StockAlertsPage = lazy(() => import("./pages/StockAlertsPage").then((m) => ({ default: m.StockAlertsPage })));
const LoansPage = lazy(() => import("./pages/LoansPage").then((m) => ({ default: m.LoansPage })));
const MaintenancePage = lazy(() => import("./pages/MaintenancePage").then((m) => ({ default: m.MaintenancePage })));
const SuppliersPage = lazy(() => import("./pages/SuppliersPage").then((m) => ({ default: m.SuppliersPage })));
const ReorderSuggestionsPage = lazy(() => import("./pages/ReorderSuggestionsPage").then((m) => ({ default: m.ReorderSuggestionsPage })));
const PurchaseOrdersPage = lazy(() => import("./pages/PurchaseOrdersPage").then((m) => ({ default: m.PurchaseOrdersPage })));
const PurchaseOrderDetailPage = lazy(() => import("./pages/PurchaseOrderDetailPage").then((m) => ({ default: m.PurchaseOrderDetailPage })));
const MorePage = lazy(() => import("./pages/MorePage").then((m) => ({ default: m.MorePage })));
const LabelsPage = lazy(() => import("./pages/LabelsPage").then((m) => ({ default: m.LabelsPage })));
const UsersPage = lazy(() => import("./pages/UsersPage").then((m) => ({ default: m.UsersPage })));
const AccountPage = lazy(() => import("./pages/AccountPage").then((m) => ({ default: m.AccountPage })));
const CategoriesAdminPage = lazy(() => import("./pages/CategoriesAdminPage").then((m) => ({ default: m.CategoriesAdminPage })));
const LocationsAdminPage = lazy(() => import("./pages/LocationsAdminPage").then((m) => ({ default: m.LocationsAdminPage })));
const ScanPage = lazy(() => import("./pages/ScanPage").then((m) => ({ default: m.ScanPage })));
const ResolveCodePage = lazy(() => import("./pages/ResolveCodePage").then((m) => ({ default: m.ResolveCodePage })));

const RolesPage = lazy(() => import("./pages/RolesPage").then((m) => ({ default: m.RolesPage })));

const CustomersPage = lazy(() => import("./pages/CustomersPage").then((m) => ({ default: m.CustomersPage })));
const ProjectsPage = lazy(() => import("./pages/ProjectsPage").then((m) => ({ default: m.ProjectsPage })));
const ProjectDetailPage = lazy(() => import("./pages/ProjectDetailPage").then((m) => ({ default: m.ProjectDetailPage })));

export function App() {
  return (
    <>
      <OfflineBanner />
      <Suspense fallback={<LoadingState label="Cargando" />}>
        <Routes>
          <Route path="login" element={<LoginPage />} />
          <Route element={<RequireAuth />}>
            <Route element={<AppShell />}>
              <Route index element={<Navigate to="/inicio" replace />} />
              <Route element={<RequireRole check={(p) => p.canRead} />}>
              <Route path="inicio" element={<DashboardPage />} />
              <Route path="materiales" element={<ItemsPage />} />
              <Route path="materiales/codigo/:code" element={<ResolveCodePage />} />
              <Route element={<RequireRole check={(p) => p.canWriteItems} />}>
                <Route path="materiales/etiquetas" element={<LabelsPage />} />
              </Route>
              <Route path="materiales/:id" element={<ItemDetailPage />} />
              <Route path="stock-bajo" element={<StockAlertsPage />} />
              <Route path="prestamos" element={<LoansPage />} />
              <Route path="mantenimiento" element={<MaintenancePage />} />
              <Route path="proveedores" element={<SuppliersPage />} />
              <Route path="ordenes-compra" element={<PurchaseOrdersPage />} />
              <Route path="ordenes-compra/sugerencias" element={<ReorderSuggestionsPage />} />
              <Route path="ordenes-compra/:id" element={<PurchaseOrderDetailPage />} />
              <Route path="escanear" element={<ScanPage />} />
              </Route>
              <Route element={<RequireRole check={(p) => p.canReadCustomers} />}>
                <Route path="clientes" element={<CustomersPage />} />
              </Route>
              <Route element={<RequireRole check={(p) => p.canReadProjects} />}>
                <Route path="proyectos" element={<ProjectsPage />} />
                <Route path="proyectos/:id" element={<ProjectDetailPage />} />
              </Route>
              <Route path="mas" element={<MorePage />} />
              <Route path="cuenta" element={<AccountPage />} />
              <Route path="admin/usuarios" element={<RequireRole check={(p) => p.canManageUsers} />}><Route index element={<UsersPage />} /></Route>
              <Route path="admin/roles" element={<RequireRole check={(p) => p.canManageRoles} />}><Route index element={<RolesPage />} /></Route>
              <Route path="admin" element={<RequireRole check={(p) => p.canManageCatalogs} />}>
                <Route path="categorias" element={<CategoriesAdminPage />} />
                <Route path="ubicaciones" element={<LocationsAdminPage />} />
              </Route>
            </Route>
          </Route>
          <Route path="*" element={<Navigate to="/inicio" replace />} />
        </Routes>
      </Suspense>
    </>
  );
}
