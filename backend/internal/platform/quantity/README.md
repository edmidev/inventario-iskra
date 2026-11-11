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

## Integration

Catalog, stock, purchasing, dashboard and importer now use this value type.
Migration `00013_decimal_quantities.sql` preserves human-unit database values,
changes quantity columns to NUMERIC(16,3), and rebuilds the deferred total-check
trigger with numeric local variables. Its down migration refuses fractional
history, including embedded purchase-line item minimums, rather than rounding.
Assets remain whole units. Purchase line totals round half up before summing;
validated maximum lines, quantity and unit cost keep the order sum within int64.
