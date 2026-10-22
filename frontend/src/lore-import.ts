export function parseCSV(text: string): Record<string, string>[] {
  text = text.replace(/^\uFEFF/, "");
  const table: string[][] = [];
  let row: string[] = [], value = "", quoted = false, endedQuote = false;
  for (let i = 0; i < text.length; i++) {
    const c = text[i];
    if (quoted) {
      if (c === '"') {
        if (text[i + 1] === '"') { value += '"'; i++; }
        else { quoted = false; endedQuote = true; }
      } else value += c;
    } else if (c === '"') {
      if (value || endedQuote) throw Error("CSV 引号位置无效");
      quoted = true;
    } else if (c === "," || c === "\n" || c === "\r") {
      row.push(value); value = ""; endedQuote = false;
      if (c !== ",") {
        if (row.some((s) => s.trim())) table.push(row);
        row = [];
        if (c === "\r" && text[i + 1] === "\n") i++;
      }
    } else {
      if (endedQuote && c.trim()) throw Error("CSV 闭合引号后存在多余字符");
      if (!endedQuote) value += c;
    }
  }
  if (quoted) throw Error("CSV 含未闭合的引号");
  row.push(value); if (row.some((s) => s.trim())) table.push(row);
  if (table.length < 2) throw Error("CSV 需要表头和至少一条记录");
  const headers = table.shift()!.map((s) => s.trim());
  if (headers.some((h) => !h) || new Set(headers).size !== headers.length) throw Error("CSV 表头不能为空或重复");
  if (table.length > 200) throw Error("一次最多导入 200 条设定");
  return table.map((r, i) => {
    if (r.length !== headers.length) throw Error(`CSV 第 ${i + 2} 行列数与表头不同`);
    return Object.fromEntries(headers.map((h, n) => [h, r[n]]));
  });
}

export const loreFields = [
  ["name", "名称"], ["kind", "类型"], ["base", "基础设定"],
  ["traits", "性格/特征"], ["likes", "喜好/条件"], ["state", "当前状态"],
  ["knowledge", "已知信息"], ["note", "备注"], ["gender", "性别"],
  ["birthplace", "出生地"], ["birthday", "出生日期"], ["appearance", "外貌"],
  ["identity", "身份/职业"], ["relationships", "关系"],
] as const;
export function loreKind(value: string): "character" | "item" | "location" | undefined {
  return ({character:"character",人物:"character",角色:"character",item:"item",物品:"item",道具:"item",location:"location",场景:"location",地点:"location"} as const)[value.trim().toLowerCase() as "character"];
}
