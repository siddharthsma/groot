export type GraphNodeData = {
  label: string;
  kind: "event" | "subscription" | "connector";
};

export type GraphLayoutNode = {
  id: string;
  data: GraphNodeData;
  position: { x: number; y: number };
};
