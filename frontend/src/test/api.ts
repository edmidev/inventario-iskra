export const api = (path: string) => `http://localhost:3000/api/v1${path}`;

export function errorBody(code: string, message: string, fields?: Record<string, string>) {
  return { error: { code, message, ...(fields ? { fields } : {}) } };
}
