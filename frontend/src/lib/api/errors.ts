export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly fields: Record<string, string>;

  constructor(status: number, code: string, message: string, fields: Record<string, string> = {}) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.fields = fields;
  }
}

const messages: Record<string, string> = {
  invalid_credentials: "Correo o contraseña incorrectos.",
  rate_limited: "Demasiados intentos. Espera un momento e inténtalo de nuevo.",
  unauthorized: "Tu sesión expiró. Inicia sesión de nuevo.",
  forbidden: "No tienes permiso para realizar esta acción.",
  not_found: "No se encontró el recurso solicitado.",
  validation_failed: "Revisa los campos marcados.",
  item_on_loan: "El material está prestado. Registra la devolución primero.",
  item_under_repair: "El material tiene una falla abierta; ciérrala antes de cambiar su estado.",
  code_taken: "Ese código ya está en uso.",
  category_in_use: "La categoría todavía tiene materiales.",
  location_in_use: "La ubicación todavía tiene materiales o sububicaciones.",
  insufficient_stock: "No hay existencias suficientes.",
  network: "No se pudo conectar con el servidor. Revisa tu conexión.",
  invalid_response: "El servidor respondió con un formato inesperado.",
  internal_error: "Ocurrió un error en el servidor. Inténtalo más tarde.",
};

/** Maps any thrown value to a readable Spanish message. */
export function describeError(error: unknown): string {
  if (error instanceof ApiError) {
    return messages[error.code] ?? (error.message || "Ocurrió un error inesperado.");
  }
  return "Ocurrió un error inesperado.";
}
