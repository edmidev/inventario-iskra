/** Item codes look like ISK-HER-001: letters, digits and the separators . _ - */
const CODE_PATTERN = /^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/;
const URL_CODE_PATTERN = /\/(?:materiales\/(?:codigo|by-code)|items\/by-code)\/([^/?#]+)/i;

/**
 * Extracts an item code from whatever a QR (or the manual field) produced: a bare code
 * or a URL that carries it (`/materiales/codigo/<code>`, `/materiales/by-code/<code>`).
 * Returns null when nothing usable is found. The API matches codes case-insensitively;
 * we normalize to uppercase for display and cache keys.
 */
export function extractItemCode(raw: string): string | null {
  const text = raw.trim();
  if (text === "") return null;
  const fromUrl = URL_CODE_PATTERN.exec(text);
  if (fromUrl) {
    let segment = fromUrl[1] ?? "";
    try {
      segment = decodeURIComponent(segment);
    } catch {
      return null;
    }
    return CODE_PATTERN.test(segment) ? segment.toUpperCase() : null;
  }
  return CODE_PATTERN.test(text) ? text.toUpperCase() : null;
}

/** The URL encoded in printed labels; a plain phone camera opens it in the browser. */
export function labelUrl(origin: string, code: string): string {
  return `${origin.replace(/\/$/, "")}/materiales/codigo/${encodeURIComponent(code)}`;
}
