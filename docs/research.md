# 小说写作工具 GitHub 调研与选型

调研日期为 2026 年 10 月 11 日。目标是给 novel_edit 选择可借鉴的小说管理、写作界面和 AI 协作方案。建议独立实现 Go 桌面应用，重点参考 Goink 的结构化设定、bibisco 的小说规划和 novelWriter 的章节组织。

## 候选项目比较

下表中的现有功能以官方仓库介绍为依据。调研包含仓库文档和两个 Go 项目的入口代码阅读，未安装运行这些候选软件；界面体验、稳定性和中文输入表现需要后续实测。

| 项目与官方来源 | 技术或定位 | 值得借鉴 | 本项目需要补充 | 仓库标示许可 |
| --- | --- | --- | --- | --- |
| [Goink](https://github.com/sigpanic/goink) | Go、Wails、React、SQLite；AI 小说桌面工具 | 卷章管理、角色与关系历史、伏笔和弧线、语义搜索、修改对比 | 按阶段解析人物、物品、场景版本；更适合阅读与写作的排版编辑器 | AGPL-3.0，附 NOTICE |
| [Show Me The Story](https://github.com/Nigh/show-me-the-story) | 单 Go 可执行文件与网页界面 | 多项目、大纲批次、事实来源、设定变更提案、审稿 | 强化手工写作工作台、阶段设定侧栏与字体排版 | MIT |
| [bibisco](https://github.com/andreafeccomandi/bibisco) | 传统小说规划与写作工具 | 人物塑造、章节与场景、叙事线、修订和导出 | 本地 AI、按阶段切换的设定、上下文组装 | GPL-3.0；仓库为 Community Edition |
| [novelWriter](https://github.com/saga-soft/novelWriter) | Python、PyQt6；文本式小说编辑器 | 文稿拆分、层级组织、摘要与交叉引用、可读文件存储 | 富文本排版、分阶段设定和 AI 服务 | GPL-3.0 |
| [Vela](https://github.com/heider-x/vela) | Electron、React、TypeScript；AI 小说工作台 | 写作与 AI 并排布局、本地知识检索、模型配置、审阅工作流 | Go 一体化交付、明确的阶段版本规则 | GPL-3.0 |
| [LLM Novel Writer](https://github.com/Zhrq-vis/LLM-Novel-Writer) | FastAPI、原生网页、SQLite | 每章绑定角色与事件、前情提要、角色经历同步 | 物品与场景的历史版本、桌面交付、字体与编辑体验 | MIT |
| [Novel](https://github.com/steven-tey/novel) | 基于 Tiptap 的 Notion 风格编辑器 | 选区交互、编辑器工具栏、AI 辅助入口 | 小说项目、大纲、故事线和设定管理 | Apache-2.0 |

Novel 是编辑器项目，并非完整的长篇小说管理软件。Vela 等项目文档中的质量或一致性宣传不能作为实际效果保证。

## Go 一体化方案

[Goink 的 main.go](https://github.com/sigpanic/goink/blob/master/main.go) 使用 go:embed 嵌入 frontend/dist，再通过 Wails 启动桌面窗口并绑定 Go 方法。[Show Me The Story 的 main.go](https://github.com/Nigh/show-me-the-story/blob/main/main.go) 同样嵌入前端资源，但通过本地 HTTP 服务提供网页界面。这两种方案都能把界面与 Go 业务打包发布。

推荐采用 Wails 桌面模式，因为本项目需要长时间写作、字体设置、文件导入导出和桌面窗口体验。业务层独立于 Wails，后续可以追加 Go HTTP 入口，复用同一套数据与 AI 服务。

[Wails 官方文档](https://wails.io/docs/introduction/) 支持 Go 与现代网页前端组合，并可将前端资源打包入可执行文件。当前文档将 v3 标为 beta，首版建议使用 v2 稳定系列，初始化工程时锁定确切依赖版本。Windows 使用 WebView2，所以“一体化”不意味着完全没有操作系统运行时依赖。

本地推理首版通过外部服务接入：[Ollama 提供本地 API](https://docs.ollama.com/api/introduction)，[LM Studio 提供兼容接口](https://lmstudio.ai/docs/developer/openai-compat)。模型和推理运行时通常较大，不放入小说编辑器安装包。无需 AI 时，编辑、设定和规则检查应继续离线工作。

## 界面与编辑器选择

推荐使用 React + TypeScript 构建三栏工作台，以暖白底色、墨绿主色和中文正文排版形成统一视觉。写作页默认突出正文，右侧切换阶段资料、AI、审稿和灵感；复杂设定通过单独页面维护。

推荐 Tiptap 作为正文编辑器基础。[其 FontFamily 扩展](https://tiptap.dev/docs/editor/extensions/functionality/fontfamily) 提供字体族设置能力。字号、行距、首行缩进、主题和中文输入行为仍需本项目实现和验证。Monaco 更适合代码式文本操作；本项目优先考虑作者阅读、段落和选区排版体验。

## 代码借鉴方式

首版采用独立工程，借鉴交互和数据组织思路。若复用具体代码，应记录文件来源、版本、许可和必要声明。Goink 的 AGPL 与其他项目的 GPL 许可需结合实际复用和发布方式评估；MIT、Apache 项目也需要保留相应许可声明。当前没有复制候选项目源码。

## AI 检测的产品边界

文风评价、设定一致性检查、素材相似度检查和 AI 来源检测应分别呈现。

[Can AI Generated Text be Reliably Detected](https://arxiv.org/abs/2303.11156) 讨论了检测器在改写等情况下的脆弱性。因此，AI 来源检测应作为实验性辅助结果，标注检测器、适用语言、输入长度与限制，不承诺准确判断作者身份或平台审核结果。中文小说需要单独验证，不能套用英文论文中的准确率。

优先实现能给出原文位置、依据和改法的审稿功能，以及应用内部 AI 生成和采纳记录。它们更直接服务于作者修改文稿。
