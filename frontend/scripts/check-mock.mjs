import assert from "node:assert/strict";
const base = process.env.PRISM_URL || "http://127.0.0.1:4010";
const headers = {
  Authorization: "Bearer local-prism-placeholder",
  "Content-Type": "application/json",
};
const health = await fetch(`${base}/healthz`);
assert.equal(health.status, 200);
const uploads = await fetch(`${base}/v1/uploads`, { headers });
assert.equal(uploads.status, 200);
assert.ok(Array.isArray((await uploads.json()).items));
const incident = await fetch(
  `${base}/v1/incidents/3f2b8c1e-5d4a-4e6f-9a1b-2c3d4e5f6a7b`,
  { headers },
);
assert.equal(incident.status, 200);
const detail = await incident.json();
assert.ok(Array.isArray(detail.evidence));
assert.ok(Array.isArray(detail.signals));
const invalid = await fetch(`${base}/v1/uploads`, {
  method: "POST",
  headers,
  body: JSON.stringify({ filename: "test.log" }),
});
assert.equal(invalid.status, 422);
console.log(
  "Prism passed: health, uploads, incident detail, and invalid upload validation.",
);
