import "server-only";
import { redirect } from "next/navigation";
import { accessToken } from "@/lib/supabase/server";
import { createApiClient } from "./client";
import type { Schema } from "@/lib/types";

export function apiUrl() {
  return process.env.TENFOLD_API_URL || "https://tenfold-api.up.railway.app";
}
export async function serverApi() {
  const token = await accessToken();
  if (!token) redirect("/sign-in");
  const client = createApiClient(apiUrl(), token);
  client.use({
    onResponse({ response }) {
      if (response.status === 401) redirect("/sign-in?expired=1");
    },
  });
  return client;
}
export type ApiResult<T> =
  { data: T; error?: never } | { data?: never; error: string };
export function result<T>(response: {
  data?: T;
  error?: Schema["Problem"];
  response: Response;
}): ApiResult<T> {
  if (!response.response.ok)
    return {
      error:
        response.error?.detail ||
        response.error?.title ||
        `Request failed (${response.response.status}).`,
    };
  return { data: response.data as T };
}
export function required<T>(response: {
  data?: T;
  error?: Schema["Problem"];
  response: Response;
}): T {
  const value = result(response);
  if (value.error) throw new Error(value.error);
  return value.data as T;
}
