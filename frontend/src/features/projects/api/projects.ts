import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { z } from "zod";
import { request } from "../../../lib/api/client";
import { usePagedQuery } from "../../../lib/api/paged";
import { ApiError } from "../../../lib/api/errors";
import { itemSchema } from "../../../lib/api/schemas";
export const projectSchema = z.object({
  id: z.number(),
  name: z.string(),
  description: z.string(),
  characteristics: z.array(z.string()),
  consumables: z.array(z.object({
    item_id: z.string(),
    quantity: z.number()
  })),
  tools: z.array(z.string()),
  estimated_days: z.number().int(),
  created_at: z.string(),
  updated_at: z.string()
});
export type Project = z.infer<typeof projectSchema>;
export type ProjectPayload = Pick<Project, "name" | "description" | "characteristics" | "consumables" | "tools" | "estimated_days">;
export const projectsKey = ["projects"] as const;
export function useProjects(q: string) {
  return usePagedQuery([...projectsKey, q], "/projects", projectSchema, {
    q: q.trim() || undefined
  });
}
export function useProject(id: number) {
  return useQuery({
    queryKey: [...projectsKey, "detail", id],
    queryFn: ({
      signal
    }) => request(`/projects/${id}`, projectSchema, {
      signal
    }),
    enabled: Number.isSafeInteger(id) && id > 0
  });
}
export function useSaveProject(id?: number) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (body: ProjectPayload) => {
      try {
        return await request(id ? `/projects/${id}` : "/projects", projectSchema, {
          method: id ? "PUT" : "POST",
          body
        });
      } catch (error) {
        if (error instanceof ApiError) {
          const fields = Object.fromEntries(Object.entries(error.fields).map(([key, value]) => [key.replace(/^(tools\[\d+\])$/, "$1.item_id").replace(/^characteristics\[\d+\]$/, "characteristics"), value]));
          throw new ApiError(error.status, error.code, error.code === "project_name_taken" ? "Ya existe un proyecto con ese nombre." : error.message, fields);
        }
        throw error;
      }
    },
    onSuccess: () => client.invalidateQueries({
      queryKey: projectsKey
    })
  });
}
export function useDeleteProject(id: number) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: () => request(`/projects/${id}`, null, {
      method: "DELETE"
    }),
    onSuccess: () => client.invalidateQueries({
      queryKey: projectsKey
    })
  });
}
/** Search keeps inventory pickers usable even when the catalog exceeds one page. */
export function useTemplateItems(kind: "asset" | "consumable", q: string) {
  return usePagedQuery(["template-items", kind, q], "/items", itemSchema, {
    kind,
    q: q.trim() || undefined
  }, {
    limit: 100
  });
}
