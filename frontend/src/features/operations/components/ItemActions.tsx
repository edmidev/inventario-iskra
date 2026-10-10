import { useState } from "react";
import { ItemActionBar } from "../../../components/organisms/ItemActionBar";
import type { Item } from "../../../lib/api/schemas";
import { usePermissions } from "../../auth/session";
import { AssignSheet, ReturnSheet } from "../../custody/components/LoanSheets";
import { MovementSheet, TransferSheet } from "../../stock/components/StockSheets";
import { buildActions, type ActionId } from "../actions";

/** Action bar plus the sheet each action opens. Renders nothing for viewers. */
export function ItemActions({ item }: { item: Item }) {
  const { canOperate } = usePermissions();
  const [active, setActive] = useState<ActionId | null>(null);
  if (!canOperate) return null;

  const actions = buildActions(item, null);
  const close = (open: boolean) => {
    if (!open) setActive(null);
  };
  const quantityMode = active === "in" || active === "out" || active === "adjust" ? active : null;

  return (
    <>
      <ItemActionBar actions={actions} onSelect={setActive} />
      {quantityMode && <MovementSheet mode={quantityMode} subject={{ ...item, unit: item.unit }} open onOpenChange={close} />}
      {active === "assign" && <AssignSheet item={item} open onOpenChange={close} />}
      {active === "return" && <ReturnSheet item={item} open onOpenChange={close} />}
      {active === "transfer" && <TransferSheet item={item} open onOpenChange={close} />}
    </>
  );
}
