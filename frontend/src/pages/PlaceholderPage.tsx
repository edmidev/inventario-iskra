import { EmptyState } from "../components/molecules/StateMessage";

export function PlaceholderPage({ title, description }: { title: string; description: string }) {
  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl font-bold">{title}</h1>
      <EmptyState title="Próximamente" description={description} />
    </div>
  );
}
