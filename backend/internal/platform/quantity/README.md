# Exact quantity foundation

`Quantity` stores integer thousandths. JSON and PostgreSQL NUMERIC use human
units: `0.125` is internally `125`, but an existing quantity `5` must be
converted with `Units(5)` (internal `5000`), never with `Quantity(5)`.

- `Parse`, JSON decoding and SQL scanning reject unrepresentable precision,
  non-finite values, null and overflow. JSON remains numeric, not quoted.
- `String` and JSON encode canonical human units without trailing zeros.
- `NumericValue` / `ScanNumeric` support pgx binary and text NUMERIC codecs.
- `Add`, `Multiply` and `Cost` check overflow. `Cost` rounds a nonnegative
  quantity times integer minor-unit price half up, separately per line.
- `Whole` allows asset-only domain paths to reject fractional units.

## Activation boundary

This foundation is intentionally not wired into existing modules yet. It does
not change any current integer quantity, database column, endpoint or trigger.
Do not deploy decimal-writing endpoints until the following transition is
complete and verified together:

1. Convert quantity-bearing domain fields, requests, responses and ports in
   catalog, stock, purchasing, dashboard and importer. Preserve integer counts,
   IDs, pagination, dates and asset counts as integers.
2. Explicitly convert existing whole-unit inputs and fixtures with `Units` or
   `Parse`. Change maximum-stock limits to the corresponding scaled value.
   Preserve asset integrality and asset line limits before conversion to loops.
3. Update order line totals to use checked `Cost` and checked order summation.
4. Migrate items, item_stocks, stock_movements, purchase_order_lines,
   po_receipt_lines and po_receipt_items to decimal numeric quantities without
   scaling the SQL values. Update item_stock_total_check's INTEGER/BIGINT local
   variables to NUMERIC. Preserve ledger constraints and immutability.
5. Audit query casts/aggregate scans, JSON new_item.stock_min, importer parsing,
   report formatting and cmd/api bridge ports. Preserve whole-unit API inputs.
6. Adapt forms and OpenAPI, then verify real PostgreSQL integer-history
   preservation, fractional movements/receipts, balance trigger behavior and
   failed down-migration when any fractional value would be lost.

The current backend remains integer-only until those integration steps land.
