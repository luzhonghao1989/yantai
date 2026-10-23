export type ComfyNode = { class_type: string; inputs: Record<string, unknown>; _meta?: { title?: string } };
export function parseComfyWorkflow(text: string): Record<string, ComfyNode> {
  let nodes;
  try { nodes = JSON.parse(text); } catch { throw Error("工作流 JSON 格式无效"); }
  if (nodes?.prompt && typeof nodes.prompt === "object") nodes = nodes.prompt;
  if (!nodes || Array.isArray(nodes) || typeof nodes !== "object" || !Object.keys(nodes).length || Object.values(nodes).some(n => !n || typeof n !== "object" || typeof (n as ComfyNode).class_type !== "string" || !(n as ComfyNode).inputs || typeof (n as ComfyNode).inputs !== "object" || Array.isArray((n as ComfyNode).inputs)))
    throw Error("请在 ComfyUI 使用“导出 API”，导入 API 格式 JSON；普通画布工作流不能直接提交。");
  return nodes;
}
export function comfyTextInputs(nodes: Record<string, ComfyNode>) {
  return Object.entries(nodes).flatMap(([id, node]) => Object.entries(node.inputs).filter(([, value]) => typeof value === "string").map(([field]) => ({ id, field, label: `${id} · ${node._meta?.title || node.class_type} / ${field}` })));
}
export function inferComfyBindings(nodes: Record<string, ComfyNode>) {
  const fields = comfyTextInputs(nodes);
  const byLink = (key: string) => {
    for (const node of Object.values(nodes)) {
      const value = node.inputs[key];
      if (Array.isArray(value)) { const names = key === "negative" ? ["negative_prompt", "text"] : ["prompt", "positive_prompt", "text"]; for (const field of names) { const entry = fields.find(f => f.id === String(value[0]) && f.field === field); if (entry) return entry; } }
    }
  };
  const positive = byLink("positive") || fields.find(f => ["text", "prompt", "positive_prompt"].includes(f.field));
  const negative = byLink("negative");
  return { positiveNode: positive?.id || "", positiveInput: positive?.field || "text", negativeNode: negative?.id || "", negativeInput: negative?.field || "text", outputNode: Object.entries(nodes).find(([,n]) => n.class_type === "SaveImage")?.[0] || "" };
}
