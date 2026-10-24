# 服务端部署与多设备同步

公网端口只接受 Yantai 客户端的 TLS 加密同步请求，不提供网页、登录页、模型设置或 Agent API。普通浏览器访问、无效密钥和非同步请求返回空的 404 响应。端口仍然可以被网络扫描发现，空响应不代表端口不可见。

当前面向个人多设备，共享一个所有者访问密钥，不提供多人账户与权限分组。

## Linux amd64

```sh
chmod +x Yantai-server
export YANTAI_SERVER_TOKEN='替换为至少16字符的随机访问密钥'
./Yantai-server -server -listen 0.0.0.0:48120 -data ./data
```

启动时自动在 `data/tls` 生成并保存私有 TLS 证书与私钥，输出证书 SHA-256 指纹。通过服务器终端核对指纹，再填写到客户端。不要从未验证的公网连接自动接受证书。证书在重启后保持不变，升级请保留整个 data 目录，包括 tls。不要分享或提交私钥。

可选自有域名证书：增加 `-tls-cert fullchain.pem -tls-key privkey.pem`。客户端正常验证可信 CA 和域名，证书指纹可以留空；填写指纹时必须严格匹配指定证书，证书更新后也需重新核对指纹。

程序内置 SQLite，无需 Node.js 或另装数据库。防火墙只需开放同步端口。systemd 模板见 `deploy/yantai.service`：建立 yantai 用户，程序放在 `/opt/yantai/Yantai-server`，数据目录 `/var/lib/yantai`，访问密钥放在 `/etc/yantai/server.env` 并限制读取权限。执行 `systemctl daemon-reload` 和 `systemctl enable --now yantai`。

## systemd 服务安装与更新

新版服务文件为 `deploy/yantai.service`，只启动 TLS 同步端口，默认不启用网页。systemd 自动创建 `/var/lib/yantai` 并归属 yantai 用户，目录权限 0700、文件默认权限 0600。自动生成的证书在 `/var/lib/yantai/tls`，更新时必须保留。

首次安装需先创建 yantai 系统用户，安装程序至 `/opt/yantai/Yantai-server` 并赋予执行权限。建立 `/etc/yantai/server.env`，内容为 `YANTAI_SERVER_TOKEN=你的随机访问密钥`，将文件设为 root 所有、权限 0600。已部署的服务沿用原密钥与数据，不要重新生成或覆盖。

在解压后的 Linux 发布目录执行：

```sh
sudo install -m 0644 deploy/yantai.service /etc/systemd/system/yantai.service
sudo systemctl daemon-reload
sudo systemctl enable yantai
sudo systemctl restart yantai
sudo systemctl status yantai --no-pager
sudo journalctl -u yantai -n 30 --no-pager
```

日志中的 `TLS证书 SHA-256 指纹` 用于客户端核对。客户端填写 `https://服务器IP:48120`、原访问密钥和该指纹。公网端口访问没有网页是正常现象。

从旧版升级时，还必须将程序替换为支持 TLS 专用同步的新版，单独更换服务文件不会改变旧程序行为。原服务如有自定义数据路径或端口，请同步调整 ExecStart；StateDirectory 和 ReadWritePaths 也需与实际数据目录一致。无需同时运行旧服务，也不要删除原数据目录。

## Windows Server amd64

```powershell
$env:YANTAI_SERVER_TOKEN = '替换为至少16字符的随机访问密钥'
.\Yantai-server.exe -server -listen 0.0.0.0:48120 -data D:\Yantai\data
```

同样自动生成证书并输出指纹。可用任务计划程序开机启动，为运行账户设置相同环境变量与数据目录。升级只替换程序，保留 data 和证书。

## 独立内网 HTTP 网页端口

公网同步端口仍为 `48120`，仅提供 TLS 加密同步；内网网页使用另一个端口，例如 `48121`。两者共用同一数据目录与数据库，不需要启动第二个服务进程。

启动参数示例（将 IP 改为服务器实际内网 IP）：

```sh
./Yantai-server -server -listen 0.0.0.0:48120 -web-listen 172.16.1.200:48121 -data /data/yantai/data
```

也可在 systemd 使用的 `/etc/yantai/server.env` 中增加一行：

```ini
YANTAI_WEB_LISTEN=172.16.1.200:48121
```

服务文件的 ExecStart 不需要再追加参数，程序会读取该环境变量；显式 `-web-listen` 参数优先。修改环境文件后执行 `sudo systemctl restart yantai`。若使用自定义 run.sh，则在脚本中 export 此环境变量，或直接添加 -web-listen 参数。

内网电脑打开 `http://172.16.1.200:48121`，用同一个访问密钥登录，即可使用完整网页工作台。允许绑定具体的 RFC1918 内网 IP、IPv6 ULA 或本机回环地址；不接受公网 IP、通配地址 0.0.0.0/:: 或普通域名。程序还按实际连接来源限制内网/回环地址，不信任 X-Forwarded-For 等转发头作为放行依据。

防火墙只向可信内网开放 48121，不做公网映射，也不要将该 HTTP 端口反向代理到公网。已有公网 Nginx 仍然只代理 `https://172.16.1.200:48120`。HTTP 网页仅用于可信内网，公网客户端始终通过 HTTPS 同步。

不配置 web-listen 或 YANTAI_WEB_LISTEN 时网页关闭；仅本机使用可填写 `127.0.0.1:48121`。不加 -server 仍是原有本机浏览器版，只允许回环监听。

## 连接与加密

公网同步端口始终直接使用 TLS（最低 TLS 1.2），不提供明文 HTTP 模式、HTTP 跳转或 Cookie 登录。客户端只允许 HTTPS 地址，旧版保存的 HTTP 地址也会在发请求前拒绝，需改为 HTTPS。重定向不会携带访问密钥继续请求。

私有证书使用严格的 SHA-256 证书指纹核验，并检查证书有效期；不填写指纹则使用系统信任库和域名校验。指纹不同或证书过期会拒绝连接，不会静默降级。

## Yantai.exe 客户端

1. 打开“模型与设置” → “服务端同步”。
2. 填写 `https://服务器IP:48120` 或 HTTPS 域名，不要附加路径。填写访问密钥；自动生成的私有证书还需填写服务端终端输出的 SHA-256 指纹。
3. 保存同步设置，点击“立即同步”。首次上传本机独有项目、下载服务端独有项目，保留项目 ID。
4. 可勾选“启用定时同步”，设置间隔分钟并保存。只在客户端运行期间执行。

同步范围是所有项目的当前正文、富文本格式、卷章、大纲、细纲、故事线、阶段设定、素材、AI 记录和图片。模型/API 密钥、ComfyUI 地址、同步密钥、字体主题偏好不随项目同步。历史版本分别留在各端，每次覆盖前会在目标端保留“同步前版本”。删除不自动传播。

每次先保存本窗口修改，再同步本机数据库。下载涉及当前打开项目时，若没有新编辑会自动刷新；若同步期间继续编辑，则保留窗口中的新稿并提示先导出草稿，再读取新版本，旧版本写入也会被数据库拒绝。

## 冲突

根据上次成功同步的内容校验值判断变化：只有本机改动则上传，只有服务端改动则下载，两端一致则不重复写入。两端都改动，或首次连接时同一 ID 内容不同，则提示冲突并不覆盖任何一端。

设置页可选择“保留本机，上传”或“使用服务端，下载”。按钮按当时两端校验值执行；若选择后内容又变化，会重新报冲突。替换前的版本可在目标端“保存历史”中恢复。这是项目级同步，不是实时多人协作或逐句自动合并。

## 从源码构建

Windows 开发机运行 `scripts/build-server.ps1`，可同时生成 Windows 与 Linux amd64 服务端。Linux 开发机先构建前端，再运行 `CGO_ENABLED=0 go build -o Yantai-server ./cmd/server`。服务端不依赖 Wails 图形运行环境。
