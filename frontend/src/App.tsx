import { lazy, Suspense } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
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
const MorePage = lazy(() => import("./pages/MorePage").then((m) => ({ default: m.MorePage })));
const LabelsPage = lazy(() => import("./pages/LabelsPage").then((m) => ({ default: m.LabelsPage })));
const ScanPage = lazy(() => import("./pages/ScanPage").then((m) => ({ default: m.ScanPage })));
const ResolveCodePage = lazy(() => import("./pages/ResolveCodePage").then((m) => ({ default: m.ResolveCodePage })));

export function App() {
  return (
    <Suspense fallback={<LoadingState label="Cargando" />}>
      <Routes>
        <Route path="login" element={<LoginPage />} />
        <Route element={<RequireAuth />}>
          <Route element={<AppShell />}>
            <Route index element={<Navigate to="/inicio" replace />} />
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
            <Route path="escanear" element={<ScanPage />} />
            <Route path="mas" element={<MorePage />} />
          </Route>
        </Route>
        <Route path="*" element={<Navigate to="/inicio" replace />} />
      </Routes>
    </Suspense>
  );
}
