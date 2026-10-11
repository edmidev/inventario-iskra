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
  insufficient_stock: "No hay existencias suficientes para esa operación en esa ubicación.",
  location_required: "Indica en qué ubicación se aplica el movimiento.",
  not_applicable: "Esta operación no aplica a este tipo de material.",
  already_assigned: "El material ya está prestado. Registra su devolución primero.",
  not_assigned: "El material no está prestado.",
  item_not_available: "Solo se pueden prestar materiales funcionales. Revisa su estado.",
  open_failure_exists: "El material ya tiene una falla abierta. Actualízala o ciérrala.",
  record_closed: "Ese registro ya está cerrado y no se puede modificar.",
  invalid_request: "La solicitud no es válida. Revisa los datos e inténtalo de nuevo.",
  email_taken: "Ya existe un usuario con ese correo.",
  last_admin: "No puedes desactivar ni quitar el rol al último administrador activo.",
  invalid_current_password: "La contraseña actual no es correcta.",
  category_name_taken: "Ya existe una categoría con ese nombre.",
  category_prefix_taken: "Ese prefijo ya lo usa otra categoría.",
  location_cycle: "Esa ubicación padre crearía un ciclo.",
  conflict: "La operación entra en conflicto con el estado actual. Actualiza e inténtalo de nuevo.",
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
