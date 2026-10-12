import type { ApiResult } from "./server";
export function unwrap<T>(result: ApiResult<T>): T {
  if (result.error) throw new Error(result.error);
  return result.data as T;
}
