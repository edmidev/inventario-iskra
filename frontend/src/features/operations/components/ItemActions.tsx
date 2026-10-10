import { useState } from "react";
import { ItemActionBar } from "../../../components/organisms/ItemActionBar";
import type { Item } from "../../../lib/api/schemas";
import { usePermissions } from "../../auth/session";
import { AssignSheet, ReturnSheet } from "../../custody/components/LoanSheets";
import { useOpenFailure } from "../../maintenance/api/maintenance";
import {
  CloseFailureSheet,
  PreventiveSheet,
  ReportFailureSheet,
  UpdateFailureSheet,
} from "../../maintenance/components/MaintenanceSheets";
import { MovementSheet, TransferSheet } from "../../stock/components/StockSheets";
import { buildActions, type ActionId } from "../actions";

/** Action bar plus the sheet each action opens. Renders nothing for viewers. */
export function ItemActions({ item }: { item: Item }) {
  const { canOperate } = usePermissions();
  const [active, setActive] = useState<ActionId | null>(null);
  const failures = useOpenFailure(item.id, canOperate && item.kind === "asset");
  if (!canOperate) return null;

  const openFailure = failures.data?.pages[0]?.data[0] ?? null;
  const actions = buildActions(item, openFailure);
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
      {active === "report-failure" && <ReportFailureSheet item={item} open onOpenChange={close} />}
      {active === "preventive" && <PreventiveSheet item={item} open onOpenChange={close} />}
      {active === "update-failure" && openFailure && <UpdateFailureSheet record={openFailure} open onOpenChange={close} />}
      {active === "close-failure" && openFailure && <CloseFailureSheet record={openFailure} open onOpenChange={close} />}
      {active === "transfer" && <TransferSheet item={item} open onOpenChange={close} />}
    </>
  );
}
