import type { Upload, Schema } from './types';

export const uploadId = '7c9e6679-7425-40de-944b-e07fc1f90ae7';
export const incidentId = '3f2b8c1e-5d4a-4e6f-9a1b-2c3d4e5f6a7b';
export const completedUpload: Upload = {
  id: uploadId, filename: 'northwind-zia-2026-10-08.log', source: 'sample', sampleName: 'attack-day',
  status: 'completed', stage: 'done', progress: 100, sizeBytes: 24195844,
  lineCount: 48002, eventCount: 48002, anomalyCount: 214, incidentCount: 3,
  userCount: 24, hostCount: 31, riskThreshold: 30,
  createdAt: '2026-10-09T11:02:00Z', startedAt: '2026-10-09T11:02:01Z', completedAt: '2026-10-09T11:02:48Z',
  timeRange: { start: '2026-10-08T04:00:00Z', end: '2026-10-09T03:59:59Z' },
  verdictCounts: { open: 3, confirmed: 0, dismissed: 0, escalated: 0 },
  pipeline: [
    { stage: 'parsing', state: 'done', countIn: 48002, processed: 48002, countOut: 48002, elapsedMs: 4000 },
    { stage: 'detecting', state: 'done', countIn: 48002, processed: 48002, countOut: 214, elapsedMs: 11000 },
    { stage: 'judging', state: 'done', countIn: 214, processed: 214, countOut: 9, elapsedMs: 24000 },
    { stage: 'grouping', state: 'done', countIn: 9, processed: 9, countOut: 3, elapsedMs: 1000 },
    { stage: 'narrating', state: 'done', countIn: 3, processed: 3, countOut: 3, elapsedMs: 7000 },
  ],
};
export const processingUpload: Upload = {
  ...completedUpload, id: '8c9e6679-7425-40de-944b-e07fc1f90ae7', filename: 'northwind-zia-2026-10-09.log',
  source: 'upload', sampleName: undefined, status: 'processing', stage: 'judging', progress: 68,
  createdAt: '2026-10-09T12:41:00Z', startedAt: '2026-10-09T12:41:00Z', completedAt: undefined,
  lineCount: 51037, eventCount: 51037, anomalyCount: 198, incidentCount: undefined, verdictCounts: undefined,
  pipeline: [
    { stage: 'parsing', state: 'done', countIn: 51037, processed: 51037, countOut: 51037, elapsedMs: 4000 },
    { stage: 'detecting', state: 'done', countIn: 51037, processed: 51037, countOut: 198, elapsedMs: 11000 },
    { stage: 'judging', state: 'running', countIn: 198, processed: 132, countOut: 4 },
    { stage: 'grouping', state: 'pending' }, { stage: 'narrating', state: 'pending' },
  ],
};
export const initialUploads: Upload[] = [processingUpload, completedUpload,
  { ...completedUpload, id: '9c9e6679-7425-40de-944b-e07fc1f90ae7', filename: 'northwind-zia-2026-10-07.log', source: 'upload', sampleName: undefined, createdAt: '2026-10-08T11:05:00Z', lineCount: 46880, anomalyCount: 187, incidentCount: 0, verdictCounts: { open: 0, confirmed: 0, dismissed: 0, escalated: 0 } },
];
export const sample: Schema['Sample'] = { name: 'attack-day', title: 'Northwind Labs, attack day', description: 'One day of proxy traffic with four planted attacks and three benign decoys.', lineCount: 48002, sizeBytes: 24195844, hasAnswerKey: true };
