// The API names its failure event "error". EventSource uses the same name
// for a dropped connection. Only the API's event carries data.
export function isJobFailure(event: Event): boolean {
  return event instanceof MessageEvent && Boolean(event.data);
}
