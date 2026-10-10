import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { request } from "../../../lib/api/client";
import { listOf, userSchema, type Role } from "../../../lib/api/schemas";

export const usersKey = ["users"] as const;

export function useUsers() {
  return useQuery({
    queryKey: usersKey,
    queryFn: ({ signal }) => request("/users", listOf(userSchema), { signal }).then((r) => r.data),
  });
}

export interface UserCreatePayload {
  name: string;
  email: string;
  password: string;
  role: Role;
}

export interface UserUpdatePayload {
  name?: string;
  role?: Role;
  active?: boolean;
}

export function useCreateUser() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (payload: UserCreatePayload) => request("/users", userSchema, { method: "POST", body: payload }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: usersKey }),
  });
}

export function useUpdateUser(id: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (payload: UserUpdatePayload) => request(`/users/${id}`, userSchema, { method: "PATCH", body: payload }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: usersKey }),
  });
}

export function useResetPassword(id: string) {
  return useMutation({
    mutationFn: (newPassword: string) =>
      request(`/users/${id}/password`, null, { method: "POST", body: { new_password: newPassword } }),
  });
}
