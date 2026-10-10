import { Navigate, Route, Routes } from "react-router-dom";
import { AppShell } from "./components/organisms/AppShell";
import { RequireAuth } from "./features/auth/RequireAuth";
import { DashboardPage } from "./pages/DashboardPage";
import { ItemDetailPage } from "./pages/ItemDetailPage";
import { ItemsPage } from "./pages/ItemsPage";
import { LoginPage } from "./pages/LoginPage";
import { LoansPage } from "./pages/LoansPage";
import { MaintenancePage } from "./pages/MaintenancePage";
import { MorePage } from "./pages/MorePage";
import { PlaceholderPage } from "./pages/PlaceholderPage";
import { StockAlertsPage } from "./pages/StockAlertsPage";

export function App() {
  return (
    <Routes>
      <Route path="login" element={<LoginPage />} />
      <Route element={<RequireAuth />}>
        <Route element={<AppShell />}>
          <Route index element={<Navigate to="/inicio" replace />} />
          <Route path="inicio" element={<DashboardPage />} />
          <Route path="materiales" element={<ItemsPage />} />
          <Route path="materiales/:id" element={<ItemDetailPage />} />
          <Route
            path="escanear"
            element={<PlaceholderPage title="Escanear" description="El escaneo de códigos QR llegará en un paso posterior." />}
          />
          <Route path="stock-bajo" element={<StockAlertsPage />} />
          <Route path="prestamos" element={<LoansPage />} />
          <Route path="mantenimiento" element={<MaintenancePage />} />
          <Route path="mas" element={<MorePage />} />
        </Route>
      </Route>
      <Route path="*" element={<Navigate to="/inicio" replace />} />
    </Routes>
  );
}
