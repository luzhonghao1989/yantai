# AI Agent 写入接口

启动桌面版后，程序会在 `build/bin/data/agent-connection.json` 写入当前连接信息：`base` 是本机地址，`token` 是会话令牌。若设置 `YANTAI_DATA`，文件在对应数据目录。退出时移除文件，每次启动令牌和端口都会变化。Agent 只需读这个本机文件，不需要操作界面或数据库。不要提交或分享这个文件。

所有桌面请求携带 `X-Yantai-Token`，写请求还需 `Content-Type: application/json`。仅支持本机访问，请求体上限 64 MiB。浏览器独立版地址默认是 `http://127.0.0.1:48120`，不要求桌面令牌；不要同时用两个程序打开同一数据目录。

## 接口列表

|方法与路径|用途|
|---|---|
|GET `/api/projects`|列出小说项目|
|POST `/api/projects`|创建项目，提交 `{ "name": "小说名", "demo": false }`|
|GET `/api/projects/{id}`|读取完整项目、当前 revision 和所有记录 ID|
|POST `/api/projects/{id}/agent/apply`|按 ID 增量更新，一次请求保存一个完整历史版本|
|PUT `/api/projects/{id}`|用完整项目 JSON 保存；适合删除、排序和富文本操作|
|GET `/api/projects/{id}/history`|读取历史列表|
|POST `/api/projects/{id}/restore`|提交 `{ "revision": 旧版本号, "expected": 当前版本号 }` 恢复|
|GET `/api/projects/{id}/export?format=json`|导出可重新导入的完整项目|
|POST `/api/import`|导入完整项目 JSON，生成独立项目|

## 增量写入规则

顶层必须提交当前整数 `revision`。可写字符串字段为 `name`、`genre`、`description`、`outline`、`rules`、`style`、`banned`；集合字段为 `volumes`、`stages`、`chapters`、`entities`、`storylines`、`sources`。

集合按记录 `id` 更新，未提交的记录及记录字段保留。新记录可以省略 ID，由服务生成；若一次批量建立互相引用的新记录，应自行生成唯一的 32 位十六进制 ID。响应返回完整保存后的项目及新 revision。

记录内部的数组和 `profile` 对象按提交值整体替换，包括 `versions`、`beats`、`entityIds`、`excludedIds`。因此更新人物阶段版本时，先读取现有 versions，合并后提交整个版本数组。空集合 `[]` 不会删除已有记录；删除和排序使用完整项目 PUT。

章节正文提交纯文本 `text`，服务自动生成编辑器格式；不要提交 `content`。省略 text 会保留既有正文及格式，提交空字符串会清空正文。写入前会校验关联 ID，任何失败都不保存。

并发修改导致版本不符时返回 **409**，Agent 应重新读取并比较、合并后再提交，不要盲目重试旧内容。其他请求错误返回 **400**，令牌错误返回 **403**，错误响应为 `{ "error": "说明" }`。界面已打开该项目时，点击顶部“读取 Agent 最新修改”按钮加载新内容；若有未保存草稿，先保存或导出，避免覆盖。

## 常用记录字段

|集合|字段|
|---|---|
|volumes|id、name、outline、entityIds|
|stages|id、name、volumeId、outline、entityIds|
|chapters|id、title、volumeId、stageId、outline、summary、text、entityIds、excludedIds|
|entities|id、kind（character/item/location）、name、base、profile、global、versions|
|storylines|id、name、color、beats|
|sources|id、title、text、url|

`versions` 每项包含 stageId、traits（性格/特征）、likes、state、knowledge、note。`beats` 每项包含唯一 id、title、chapterId（可为空）、status（planned/planted/resolved）、detail。

人物 `profile` 是字符串字典，常用键为 gender（性别）、birthplace（出生地）、birthday（出生日期）、ethnicity（种族/民族）、appearance（外貌）、family（家庭）、identity（身份/职业）、affiliation（势力）、relationships（关系）。仅启用的字段保存，空字符串表示已勾选但尚未填写；随时期变化的资料建议放在阶段版本中。

## Python 调用示例

下面使用标准库，直接修改第一部小说的总纲、第一章章纲，并新增第二章。调用前请先在界面新建小说。

```python
import json
from pathlib import Path
from urllib.request import Request, urlopen

connection = json.loads(Path(
    r"E:\04-script\03-tu-program\novel_edit\build\bin\data\agent-connection.json"
).read_text(encoding="utf-8"))

def request(path, payload=None):
    data = None if payload is None else json.dumps(payload, ensure_ascii=False).encode("utf-8")
    req = Request(connection["base"] + "/api" + path, data=data, headers={
        "Content-Type": "application/json",
        "X-Yantai-Token": connection["token"],
    })
    with urlopen(req) as response:
        return json.load(response)

books = request("/projects")
project = request("/projects/" + books[0]["id"])
chapter = project["chapters"][0]
saved = request("/projects/" + project["id"] + "/agent/apply", {
    "revision": project["revision"],
    "outline": "这里填写与 Agent 构思好的总纲。",
    "chapters": [
        {"id": chapter["id"], "outline": "第一章细纲。"},
        {"title": "第二章 · 渡口", "volumeId": chapter["volumeId"],
         "stageId": chapter["stageId"], "outline": "第二章细纲。",
         "text": "这里填写正文第一段。\n这里填写第二段。"},
    ],
})
print("保存成功，版本：", saved["revision"])
```

给 Agent 的建议指令：先读取连接信息和完整项目，列出目标项目名称确认工作对象；按当前 ID 写入构思结果，保持人物阶段及资料绑定；遇到 409 重新读取差异，写入后核对返回值。不直接改 novel.db。


文风参考接口：`POST /api/projects/{id}/style-reference`，请求包含 `revision`、`chapterId`、`query`（不超过 1000 字节）。强制执行联网检索，返回 `queries`、`results`（真实网页摘要与来源）和 `analysis`（AI 写法分析与原创示例）。使用已保存的模型配置，未配置 Brave 密钥时采用免密钥搜索。


语义检索：`POST /api/projects/{id}/semantic-search`，请求 `revision`、`chapterId`、`query`，返回按相似度排序的真实 `SearchResult`；`score` 为余弦相似度乘以 1000 后的整数。需在全局设置填写 `embeddingModel`。已有导出接口的 `format` 增加 `docx`、`epub`，返回二进制文件。

## v0.4 总览标签与正文空白行

大纲总览的「全书总览」「世界规则」「文风约定」分别对应项目顶层 `outline`、`rules`、`style`。切换标签仅改变显示；schema 仍为 1，增量写入路径和字段不变。更新单个标签只需提交当前 revision 和对应字段，不要清空另外两个字段。

正文工具栏「去除空白行」仅处理当前编辑章节，保留非空行的文本、字体与强调格式，可用 Ctrl+Z 一次撤销，之后沿用自动保存。导入和 Agent 写入不会自动删除空行；本次未增加服务端清理接口。需要保留富文本的 Agent 操作应使用完整项目 PUT，不能用 text 重建代替无损格式编辑。

正文修改标记存于 chapters[].content 的内联 marks：type=revisionNote，attrs 包含 id（字符串）、note（说明）、resolved（布尔值）。同一标记可能因格式或编辑分成多个片段，需按 id 处理。只更新章纲等资料时省略 text/content 即可保留；提交 text 会重新生成富文本并清除标记和字体格式，保留标记的正文编辑应读取最新完整项目后使用 PUT，并保留既有 marks。JSON 备份携带标记；文稿导出不包含修改说明。

生成接口新增 mode=map：同样提交当前 revision、chapterId、instruction，使用当前阶段上下文，流式结果为场景平面图 JSON（title、note、nodes、edges）。nodes 的 id 唯一，kind 为 room/place，x/y 范围 0—100；edges 以 from/to 引用节点，可含 label 与 uncertain。前端校验后用固定模板绘制 SVG，不执行模型返回的 HTML/SVG。

生成接口支持 mode=revision：instruction 中传入待修改标记的 id、quote（原文）、note（修改意见）及补充要求，返回 `{"replacements":[{"id":"标记ID","text":"修改后正文"}]}`。该接口只生成候选，不直接修改项目。软件界面会校验每个请求 ID 恰好出现一次、改文非空，核对生成时的正文和上下文，再按文档位置逆序替换各标记范围并将标记设为已处理；未勾选范围保持原样。

ComfyUI 生图使用独立接口，均沿用本机会话认证：

- 设置 `comfy`：baseUrl、workflow（API 格式 JSON 字符串）、positiveNode/positiveInput、negativeNode/negativeInput（可选）、outputNode（可选）、apiKey/hasKey。公开设置不返回密钥；hasKey=true 且 apiKey 留空时保留已有值。其余模型配置必须读改写保留。
- `POST /api/comfyui/test`：验证已保存配置并请求 `/system_stats`；不提交生图。
- `POST /api/projects/{id}/scene-prompt`：提交 `{chapterId,revision,instruction?,entityId?}`。instruction 是可选的画面要求，entityId 为本章当前有效场景的 ID（可省略自动选景）。文字模型读取完整章节上下文，返回 `{title,description,prompt,negativePrompt}`；校验 revision、非空正文、有效场景与模型上下文预算，失败不会请求 ComfyUI。
- `POST /api/comfyui/jobs`：`{"prompt":"画面描述","negative":"不希望出现的内容","seed":123}`，seed 可省略；返回 202 和 `{id,seed}`。ID 是 Yantai 本进程内的任务 ID，不是 ComfyUI prompt_id；重启后可在 ComfyUI 查看原任务。
- `GET /api/comfyui/jobs/{id}`：返回 `{status:"waiting",seed}` 或 `{status:"completed",seed,images:[ProjectImage...]}`。每张图片携带可保存的 DataURL、MIME、大小及 sourceRunId。上游失败、无图片或图片超限时返回明确错误。
- 生图接口不直接修改项目。界面会将结果去重添加到当前项目的 images 并通过完整 PUT 保存；Agent 调用时需保留所有已有项目数据、检查 revision 冲突，完成相同保存步骤。

界面的一键章节生图流程为：保存当前稿 → 调用 scene-prompt → 核对正文和上下文未变化 → 将生成的 prompt/negativePrompt 自动提交 comfyui/jobs → 轮询完成 → 保存图片。图片备注包含来源章节、选景依据、正负向提示词与种子，随项目备份保留。

接入方式参考 [ComfyUI 官方 HTTP API 示例](https://github.com/Comfy-Org/ComfyUI/blob/master/script_examples/websockets_api_example.py)：提交 `/prompt`，查询指定任务 `/history/{prompt_id}`，按图片元数据从 `/view` 取回文件。服务端代理这些请求，浏览器不直接携带 ComfyUI 访问密钥。

AI 使用记录新增可选 inputText（生成时的正文快照）；旧记录仍兼容。界面会自动将完成的审稿和地图加入 aiRuns 并保存。直接使用 /generate 的 Agent 需自行保存结果；该流式接口不会替 Agent 写项目。aiRuns 不属于 agent/apply 的可写集合，需要写入记录时先读取完整项目并按 revision 使用完整 PUT，保留已有所有记录。

审稿 mode=review 的新提示词要求返回 JSON 文本：`{summary,suggestions:[{paragraph,original,replacement,reason}]}`。summary 是可排版的审稿总结，original 是逐字引用的单个完整段落，replacement 是可直接替换的改文；无确定改法时 suggestions 可为空。界面兼容历史普通文本审稿；只对当前文档中唯一匹配的完整原段落启用替换，段落序号仅作展示，不能用过期序号猜测相同段落的位置。保存记录的字段结构仍为 AIRun.text 字符串，不迁移历史数据。

## v0.4.2 图片与品牌名称

规范认证头为 X-Yantai-Token，旧头仍兼容；连接文件名与结构保持 agent-connection.json。数据目录变量为 YANTAI_DATA，应用及二进制名称统一为 Yantai。

Project 新增可选 images 数组。每项包含 id、name、kind（map/character/scene/other）、note、sha、mime、size、createdAt，以及可选 sourceRunId（AI 地图来源记录）。导入新图时提供 data（base64 Data URL），使用最新 revision 完整 PUT；服务器验证后将原文件独立存储，返回 sha/mime/size 并移除 data。普通 GET 不携带图片字节。图片不属于 agent/apply 的集合；普通资料同步保持 images 原样。

GET /api/projects/{id}/images/{imageId} 返回原图，需要同样的本机会话认证。完整 JSON 导出会为每张图补齐 data，/api/import 在目标电脑恢复原图。图片随 data 目录移走即可保留；不要只复制 novel.db。单张最大 8 MiB，每项目最大 200 张及 32 MiB；JSON 请求上限更新为 64 MiB。

生成流新增可选 SSE 事件 `{"type":"thinking","data":"模型返回的思考文本增量"}`。它是临时生成进度，正式内容仍使用 delta，done.data.text 只含正式答案；调用方可忽略 thinking，勿将它拼进正文或 aiRuns.text。Generate 的非流式消费者也会剥离模型内嵌 think/thinking 内容，避免干扰 JSON 解析。
