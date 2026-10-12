'use client';
import { ReactFlow, Background, Controls, Handle, Position, MarkerType, type NodeProps, type Node } from '@xyflow/react';
import type { Upload } from '@/lib/types';
import { number } from '@/lib/format';
type StageNode = Node<{ title: string; detail: string; state: string; footer: string }>;
function PipelineNode({ data }: NodeProps<StageNode>) {
  return <div className={`pipeline-node ${data.state}`}><Handle type="target" position={Position.Left} /><strong>{data.title}</strong><span>{data.detail}</span><small>{data.footer}</small><Handle type="source" position={Position.Right} /></div>;
}
const nodeTypes = { stage: PipelineNode };
export function Pipeline({ upload }: { upload: Upload }) {
  const stages = ['parsing', 'detecting', 'judging', 'narrating'] as const;
  const labels = ['Parsing', 'Detecting', 'Judging', 'Writing report'];
  const judge = upload.pipeline.find(s => s.stage === 'judging');
  const nodes: StageNode[] = stages.map((stage, i) => {
    const item = upload.pipeline.find(s => s.stage === stage);
    const grouping = upload.pipeline.find(s => s.stage === 'grouping');
    const state = i === 3 && grouping?.state === 'running' ? 'running' : item?.state ?? 'pending';
    return { id: stage, type: 'stage', position: { x: i * 270, y: i === 3 ? 10 : 105 }, data: { title: `${i + 1} ${labels[i]}`, state,
      detail: i === 0 ? `${number(item?.countOut ?? upload.lineCount)} lines` : i === 1 ? `${number(item?.countOut)} anomalies` : i === 2 ? `${number(item?.processed ?? 0)} of ${number(item?.countIn ?? 0)}` : `${number(judge?.countOut ?? 0)} anomalies kept`,
      footer: state === 'done' ? `done${item?.elapsedMs !== undefined ? ` · ${item.elapsedMs / 1000} s` : ''}` : state === 'running' ? 'running now' : state === 'stopped' ? 'stopped' : 'waiting' } };
  });
  nodes.push({ id: 'incidents', type: 'stage', position: { x: 1080, y: 10 }, data: { title: 'Incidents', detail: upload.status === 'completed' ? `${number(upload.incidentCount)} ready to review` : 'ready when', state: 'pending', footer: upload.status === 'completed' ? 'analysis complete' : 'judging ends' } },
    { id: 'aside', type: 'stage', position: { x: 810, y: 200 }, data: { title: 'Set aside', detail: `${number(Math.max(0, (judge?.processed ?? 0) - (judge?.countOut ?? 0)))} so far`, state: 'pending', footer: 'reasons kept' } });
  const pairs = [['parsing', 'detecting'], ['detecting', 'judging'], ['judging', 'narrating'], ['narrating', 'incidents'], ['judging', 'aside']];
  const edges = pairs.map(([source, target]) => ({ id: `${source}-${target}`, source, target, type: 'smoothstep', animated: source === 'judging' && judge?.state === 'running', markerEnd: { type: MarkerType.ArrowClosed, color: 'var(--color-accent)' }, style: { stroke: 'var(--color-accent)', strokeWidth: 2 } }));
  return <div className="pipeline-canvas" aria-label="Analysis pipeline"><ReactFlow nodes={nodes} edges={edges} nodeTypes={nodeTypes} fitView fitViewOptions={{ padding: .06 }} nodesDraggable={false} nodesConnectable={false} panOnScroll={false} minZoom={.2} maxZoom={1.5}><Background color="var(--color-graph-dot)" gap={20} /><Controls showInteractive={false} position="bottom-right" /></ReactFlow></div>;
}
