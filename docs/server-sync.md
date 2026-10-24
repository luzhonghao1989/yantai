# 服务端部署与多设备同步

服务端运行与桌面版相同的页面，支持浏览器写作和 Yantai.exe 双向同步。当前面向个人多设备，共享一个所有者访问密钥，不提供多人账户与权限分组。

## Linux amd64

解压服务端压缩包，进入程序目录：

```sh
chmod +x Yantai-server
export YANTAI_SERVER_TOKEN='替换为至少16字符的随机访问密钥'
./Yantai-server -server -listen 0.0.0.0:48120 -data ./data
```

浏览器打开 `http://服务器IP:48120`，填写同一访问密钥登录。程序内嵌页面和 SQLite，无需 Node.js 或另装数据库。需允许客户端访问所选端口。

需要开机启动时，可使用 `deploy/yantai.service`：建立 yantai 系统用户，将程序放到 `/opt/yantai/Yantai-server`、数据目录放到 `/var/lib/yantai`，配置 `/etc/yantai/server.env` 的 `YANTAI_SERVER_TOKEN`，并限制该文件读取权限。复制服务文件后使用 `systemctl daemon-reload` 和 `systemctl enable --now yantai`。程序目录和数据目录需按服务文件路径配置，数据目录需允许 yantai 用户写入。

## Windows Server amd64

```powershell
$env:YANTAI_SERVER_TOKEN = '替换为至少16字符的随机访问密钥'
.\Yantai-server.exe -server -listen 0.0.0.0:48120 -data D:\Yantai\data
```

保持程序运行，也可通过 Windows 任务计划程序在开机时启动；为运行账户设置相同环境变量与数据路径。部署目录可自选，升级时只替换程序并保留数据目录。

## HTTPS 与模式

`-server` 才允许远程访问，并要求访问密钥。未加此参数时保留原本的本机浏览器模式。访问密钥通过环境变量提供，避免写入命令行参数和启动日志。

公网部署使用 HTTPS，可由反向代理转发到本机端口；HTTPS 反向代理后添加 `-secure-cookie`，并保留原始 Host 请求头；页面部署在域名根路径。也可直接提供 `-tls-cert fullchain.pem -tls-key privkey.pem` 启动 HTTPS。客户端使用相同 HTTPS 地址，证书必须有效。页面会使用 HttpOnly、SameSite 会话 Cookie，登录有效期为 12 小时。

## Yantai.exe 客户端

1. 打开“模型与设置” → “服务端同步”。
2. 填写服务端地址及部署时的访问密钥，点击“保存同步设置”。地址可以是 `http://192.168.1.10:48120` 或 HTTPS 域名，不要附加 `/api/sync`。
3. 点击“立即同步”。首次同步会上传本机独有项目，也会下载服务端独有项目，保留项目 ID。
4. 勾选“启用定时同步”，设置间隔分钟并保存。定时器只在客户端运行时执行，关闭程序后停止。

同步范围是所有项目的当前正文、富文本格式、卷章、大纲、细纲、故事线、阶段设定、素材、AI 记录和图片。模型/API 密钥、ComfyUI 地址、同步密钥、字体主题偏好不随项目同步。历史版本分别留在各端，每次覆盖前会在目标端保留“同步前版本”。删除不自动传播。

每次先保存本窗口修改，再同步本机数据库。下载涉及当前打开项目时，若没有新编辑会自动刷新；若同步期间继续编辑，则保留窗口中的新稿并提示先导出草稿，再读取新版本，旧版本写入也会被数据库拒绝。

## 冲突

根据上次成功同步的内容校验值判断变化：只有本机改动则上传，只有服务端改动则下载，两端一致则不重复写入。两端都改动，或首次连接时同一 ID 内容不同，则提示冲突并不覆盖任何一端。

设置页可选择“保留本机，上传”或“使用服务端，下载”。按钮按当时两端校验值执行；若选择后内容又变化，会重新报冲突。替换前的版本可在目标端“保存历史”中恢复。这是项目级同步，不是实时多人协作或逐句自动合并。

## 从源码构建

Windows 开发机运行 `scripts/build-server.ps1`，可同时生成 Windows 与 Linux amd64 服务端。Linux 开发机先构建前端，再运行 `CGO_ENABLED=0 go build -o Yantai-server ./cmd/server`。服务端不依赖 Wails 图形运行环境。
