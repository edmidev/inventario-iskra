import { Navigate, Route, Routes } from "react-router-dom";
import { AppShell } from "./components/organisms/AppShell";
import { PlaceholderPage } from "./pages/PlaceholderPage";

export function App() {
  return (
    <Routes>
      <Route element={<AppShell />}>
        <Route index element={<Navigate to="/inicio" replace />} />
        <Route path="inicio" element={<PlaceholderPage title="Inicio" />} />
        <Route path="materiales" element={<PlaceholderPage title="Materiales" />} />
        <Route path="escanear" element={<PlaceholderPage title="Escanear" />} />
        <Route path="mas" element={<PlaceholderPage title="Más" />} />
      </Route>
    </Routes>
  );
}
