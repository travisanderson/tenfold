export const number = (value?: number) => value === undefined ? "—" : new Intl.NumberFormat("en-US").format(value);
export const bytes = (value: number) => value < 1024 ? `${value} B` : value < 1048576 ? `${(value / 1024).toFixed(1)} KB` : `${(value / 1048576).toFixed(1)} MB`;
export const words = (value: string) => value.replaceAll("_", " ");
export function time(value: string, zone = "America/New_York", seconds = false) { return new Intl.DateTimeFormat("en-US", { timeZone: zone, hour: "2-digit", minute: "2-digit", ...(seconds ? { second: "2-digit" } : {}), hourCycle: "h23" }).format(new Date(value)); }
export function date(value: string, zone = "America/New_York") { return new Intl.DateTimeFormat("en-US", { timeZone: zone, month: "short", day: "numeric", hour: "2-digit", minute: "2-digit", hourCycle: "h23" }).format(new Date(value)); }
export function citationLabel(ids: string[]) { if (ids.length < 3) return ids.join(", "); const nums = ids.map(id => Number(id.slice(1))); return nums.every((n, i) => !i || n === nums[i-1] + 1) ? `${ids[0]}–${ids.at(-1)}` : `${ids[0]} + ${ids.length - 1} events`; }
export function safeError(error: unknown) { return error instanceof Error ? error.message : "Something went wrong. Please try again."; }
