const pad = (n: number) => String(n).padStart(2, "0");

/** `YYYY-MM-DD` in local time, the value format of `<input type="date">`. */
export function toDateInput(date: Date = new Date()): string {
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

/** `YYYY-MM-DDTHH:mm` in local time, the value format of `<input type="datetime-local">`. */
export function toDateTimeInput(date: Date = new Date()): string {
  return `${toDateInput(date)}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

/** The last minute of a local calendar day as ISO, so a loan due "today" is still in the future. */
export function endOfDayIso(dateInput: string): string {
  const [year, month, day] = dateInput.split("-").map(Number);
  return new Date(year, month - 1, day, 23, 59, 0).toISOString();
}

/** Midnight of a local calendar day as ISO. */
export function startOfDayIso(dateInput: string): string {
  const [year, month, day] = dateInput.split("-").map(Number);
  return new Date(year, month - 1, day, 0, 0, 0).toISOString();
}

export function dateTimeInputToIso(value: string): string {
  return new Date(value).toISOString();
}
