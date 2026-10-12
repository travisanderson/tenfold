'use client';
import { useState } from 'react';
import { ReactFlow, Background, Controls, MiniMap, Handle, Position, MarkerType, type NodeProps, type Node } from '@xyflow/react';
import type { Schema } from '@/lib/types';
type EntityNode = Node<{ label:string; sublabel?:string; flagged:boolean; cited:boolean }>;
function Entity({data}:NodeProps<EntityNode>){return <div className={`entity-node ${data.flagged?'flagged':''} ${data.cited?'cited':''}`}><Handle type="target" position={Position.Left}/><strong>{data.label}</strong><small>{data.sublabel}</small><Handle type="source" position={Position.Right}/></div>;}
const nodeTypes={entity:Entity};
export function ActivityGraph({ graph, selected, select }: { graph:Schema['IncidentGraph']; selected:string[];select:(ids:string[])=>void }){
 const [expanded,setExpanded]=useState<string[]>([]);
 const visible=graph.nodes.filter(n=>!n.neighbour||expanded.includes(n.id));
 const nodes:EntityNode[]=visible.map((n,i)=>({id:n.id,type:'entity',position:{x:n.x??i*280,y:n.y??100},data:{label:n.label,sublabel:n.sublabel,flagged:n.flagged,cited:n.eventIds.some(id=>selected.includes(id))},ariaLabel:`${n.label}, ${n.sublabel??n.type}`}));
 const edges=graph.edges.filter(e=>visible.some(n=>n.id===e.source)&&visible.some(n=>n.id===e.target)).map(e=>({id:e.id,source:e.source,target:e.target,label:e.label,type:'smoothstep',style:{stroke:e.flagged?'var(--color-flagged)':'var(--color-graph-edge)',strokeWidth:e.bytes?Math.max(1,Math.min(4,Math.log10(e.bytes)/2)):1},markerEnd:{type:MarkerType.ArrowClosed,color:e.flagged?'var(--color-flagged)':'var(--color-graph-edge)'},labelStyle:{fill:'var(--color-text-data)',fontFamily:'var(--font-mono)',fontSize:12},labelBgStyle:{fill:'var(--color-panel)'}}));
 return <><div className="section-heading"><strong>Activity graph · observed</strong><span>Click a node to select its evidence. Double-click to add its neighbours.</span><button className="small-button" onClick={()=>setExpanded(graph.nodes.filter(n=>n.neighbour).map(n=>n.id))}>Expand all neighbours</button></div><div className="activity-canvas"><ReactFlow key={expanded.join(',')} nodes={nodes} edges={edges} nodeTypes={nodeTypes} fitView fitViewOptions={{padding:.15}} minZoom={.2} maxZoom={2} nodesDraggable={false} nodesConnectable={false} onNodeClick={(_,node)=>select(graph.nodes.find(n=>n.id===node.id)?.eventIds??[])} onNodeDoubleClick={(_,node)=>setExpanded(prev=>[...new Set([...prev,...graph.edges.filter(e=>e.source===node.id||e.target===node.id).flatMap(e=>[e.source,e.target])])])}><Background color="var(--color-graph-dot)" gap={20}/><Controls showInteractive={false}/><MiniMap nodeColor="var(--color-accent)" maskColor="var(--color-panel)" style={{background:'var(--color-ground)',width:120,height:70}}/></ReactFlow></div></>;
}
