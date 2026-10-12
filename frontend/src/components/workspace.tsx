'use client';
import { createContext, useContext, useState } from 'react';
import { initialIncidents } from '@/lib/incident-fixtures';
import type { Incident, Upload } from '@/lib/types';
import { initialUploads, completedUpload, uploadId } from '@/lib/fixtures';

type Workspace = { incidents: Incident[]; setIncidents: React.Dispatch<React.SetStateAction<Incident[]>>; demo: boolean; zone: string; setZone: (zone: string) => void; uploads: Upload[]; setUploads: React.Dispatch<React.SetStateAction<Upload[]>>; selectedUploadId: string; setSelectedUploadId: (id: string) => void; loadDemo: () => Promise<Upload>; };
const Context = createContext<Workspace | null>(null);
export function WorkspaceProvider({ children, demo }: { children: React.ReactNode; demo: boolean }) {
  const [incidents, setIncidents] = useState<Incident[]>(initialIncidents);
  const [zone, setZone] = useState('America/New_York');
  const [uploads, setUploads] = useState<Upload[]>(demo ? initialUploads : []);
  const [selectedUploadId, setSelectedUploadId] = useState(uploadId);
  async function loadDemo() { const upload = { ...completedUpload, id: crypto.randomUUID(), filename: 'attack-day.log', createdAt: new Date().toISOString() }; setUploads(items => [upload, ...items]); setSelectedUploadId(upload.id); return upload; }
  return <Context.Provider value={{ incidents, setIncidents, demo, zone, setZone, uploads, setUploads, selectedUploadId, setSelectedUploadId, loadDemo }}>{children}</Context.Provider>;
}
export function useWorkspace() { const value = useContext(Context); if (!value) throw new Error('Workspace provider missing'); return value; }
