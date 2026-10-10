import { Outlet } from "react-router-dom";
import { BottomNav } from "../molecules/BottomNav";

export function AppShell() {
  return (
    <div className="min-h-screen bg-slate-50 text-slate-900">
      <main className="mx-auto max-w-3xl p-4 pb-20">
        <Outlet />
      </main>
      <BottomNav />
    </div>
  );
}
