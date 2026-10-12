import type { LogEvent } from "./types";
export function matchesQuery(event: LogEvent, query: string) {
  if (!query.trim()) return true;
  return query.split(/\s+OR\s+/i).some((term) => {
    const text = term.trim().toLowerCase();
    const colon = text.indexOf(":");
    if (colon > 0) {
      const field = text.slice(0, colon),
        value = text.slice(colon + 1);
      switch (field) {
        case "user":
          return event.principal.user.toLowerCase().includes(value);
        case "domain":
          return event.target.host.toLowerCase().includes(value);
        case "ip":
          return event.principal.srcIp.toLowerCase().includes(value);
        case "id":
          return event.id.toLowerCase() === value;
        default:
          return false;
      }
    }
    return [
      event.principal.user,
      event.principal.srcIp,
      event.target.host,
      event.target.url,
      event.id,
      event.securityResult.threatName ?? "",
    ].some((v) => v.toLowerCase().includes(text));
  });
}
