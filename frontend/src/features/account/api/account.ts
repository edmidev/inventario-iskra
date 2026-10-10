import { useMutation } from "@tanstack/react-query";
import { request } from "../../../lib/api/client";

export function useChangePassword() {
  return useMutation({
    mutationFn: (payload: { current_password: string; new_password: string }) =>
      request("/auth/password", null, { method: "POST", body: payload }),
  });
}
