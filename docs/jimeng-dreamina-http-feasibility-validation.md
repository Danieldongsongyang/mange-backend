# Dreamina 网页授权身份 + HTTP 协议适配可行性验证

> 本文档用于沉淀 Dreamina/即梦网页授权身份接入 `new-api` 后端的验证信息。后续新增发现请继续追加到本文件，而不是写入 `/private/tmp` 临时文档。

## 结论

当前主路线 **技术上可行，建议继续推进**：

- 已验证当前机器上的 Dreamina OAuth 登录态可用于一次真实远端授权调用。
- 已确认 `dreamina` CLI 明确使用 OAuth Device Flow，而不是浏览器 cookie 导入或纯本地状态。
- 已确认 CLI 二进制中存在授权、keyring、workspace、上传、提交任务、查询结果等 HTTP/SDK 层线索。
- 已确认 `new-api` 后端现有 Canvas relay、异步任务、公开 task id、上游 task id、计费预扣链路可以承接该能力。

但还不能直接进入完整实现，因为仍需抓取或复刻 Dreamina 的具体私有 HTTP 协议：

- 生成任务提交路径和请求体。
- `query_result` 请求路径、授权头、响应结构。
- 图片、视频、音频上传流程。
- 合规确认 `AigcComplianceConfirmationRequired` 的处理方式。

## 已验证证据

1. 本机安装了 `dreamina`

   路径：`/Users/a1/.local/bin/dreamina`

2. CLI 版本较新

   `~/.dreamina_cli/version.json` 显示版本为 `1.4.10`，发布日期为 `2026-06-26`。

3. CLI 登录机制

   `dreamina --help` 与 `dreamina login --help` 明确说明：

   - 登录使用 OAuth Device Flow。
   - 登录会打印 `verification_uri`、`user_code`、`device_code`。
   - legacy browser callback 和 manual-import login flow 不再使用。

4. 本机授权探针

   已运行不扣费探针：

   ```bash
   dreamina user_credit >/dev/null
   ```

   结果：退出码为 `0`。

   这说明当前机器上的网页授权身份能够被 CLI 用于真实远端授权调用。输出已丢弃，未读取或记录账号余额。

5. 本地凭证形态

   `~/.dreamina_cli` 顶层只看到：

   - `version.json`
   - `dreamina/SKILL.md`
   - `logs/`

   未看到明文 token 文件或现成 `tasks.db`。结合二进制中的 `github.com/zalando/go-keyring`、`macOSXKeychain.Get/Set/Delete`、`authsdk`、`access_token`、`refresh_token` 等线索，凭证高度可能存储于系统 Keychain/keyring。

6. 二进制协议线索

   `dreamina` 二进制中可见以下关键线索：

   - `https://jimeng.jianying.com`
   - `OAuth Device Flow` 相关字段：`access_token`、`refresh_token`、`verification_uri`、`device_code`
   - Keychain/keyring 相关：`github.com/zalando/go-keyring`、`macOSXKeychain.Get/Set/Delete`
   - 授权 HTTP 客户端：`doAuthorizedPost`
   - Workspace：`mweb_api.Workspace`、`workspace_id`
   - 资源上传：`ResourceUpload`、`UploadAuth`、`resource_id`、`resource_store`
   - 任务提交/查询：`SubmitTask`、`QueryResult`、`submit_id`、`gen_task_type`

## 后端接入判断

`new-api` 现有结构对这条路线友好：

- Canvas 请求入口已经存在：`/api/canvas/relay/videos`
- 路由会改写为 OpenAI 风格 `/v1/videos`
- `RelayTaskSubmit` 已支持异步任务提交、预扣费、公开 task id、上游 task id。
- `model.Task.PrivateData.UpstreamTaskID` 可保存 Dreamina 的真实 `submit_id`。
- `RelayTaskFetch` 可基于公开 `task_id` 查任务，再由 adaptor 转为 OpenAI video 响应。
- `VideoProxy` 可承接最终内容代理。

推荐新增或扩展一个 `dreamina_oauth` 风格 adaptor，而不是把本地 `dreamina` CLI 作为主执行器。

## 下一步最小验证

1. 优先验证 `query_result`

   使用已有历史 `submit_id`，抓取：

   ```bash
   dreamina query_result --submit_id=[REDACTED]
   ```

   目标是确认：

   - 域名和路径
   - 授权头或鉴权注入方式
   - 请求体字段
   - 响应结构
   - result URL / resource 字段位置

2. 验证 workspace/session

   抓取：

   ```bash
   dreamina session list
   ```

   目标是确认：

   - `workspace_id` 是否必传
   - 默认 session `0` 如何映射到远端 workspace

3. 验证上传流程

   用小尺寸测试图片，只验证上传，不急着生成：

   - 获取 upload token/auth
   - 上传文件到 TOS/ImageX/CDN
   - 调用 resource store
   - 得到 `resource_id` 或远端资源引用

4. 最后验证一次最低成本生成

   先选 text2image 或一个低成本模型，确认提交返回 `submit_id`，再通过 `query_result` 拉取结果。

## 风险

- Dreamina 私有协议可能有设备绑定、风控字段、签名字段或 Web 端合规确认。
- OAuth token 可能不能直接导出复用，需要实现安全账号绑定与 token refresh。
- 上传链路可能比提交/查询更复杂，是最大不确定点。
- 如果协议变动频繁，CLI 兜底执行器仍需作为备选方案保留。

## 2026-06-30 最小验证记录

### 验证 1：授权凭证和本地状态形态

已确认：

- `dreamina --help`、`dreamina login --help` 明确描述 OAuth Device Flow。
- macOS Keychain 中存在服务名为 `dreamina` 的通用密码记录。
- macOS Keychain 中还存在 `JianyingProWebInfoId`、`JianyingPro Safe Storage`、`douyin Safe Storage` 等剪映/抖音相关记录。
- 本轮只读取了 Keychain 记录的服务名/标签形态，未读取、未记录任何密码值、token 值、cookie 值。
- CLI 二进制链接了 macOS `Security.framework`，同时静态字符串包含 `github.com/zalando/go-keyring`、`macOSXKeychain.Get/Set/Delete`、`authsdk.Authorizer.Refresh`、`authsdk.Authorizer.Inject` 等符号。

判断：

- Dreamina CLI 的登录态高度确定是 OAuth token + 系统 keyring/keychain 组合，而不是浏览器 cookie 文件导入。
- 后端如果要复刻身份能力，理论上应接入 OAuth/device-flow/token refresh 语义，而不是依赖网页 cookie。

### 验证 2：`query_result` 的本地任务库门槛

已确认：

- `dreamina query_result --submit_id=<不存在的本地 id>` 会先查询本地 SQLite：

  ```sql
  SELECT * FROM `aigc_task` WHERE submit_id = ? ORDER BY `aigc_task`.`submit_id` LIMIT 1
  ```

- 如果本地 `~/.dreamina_cli/tasks.db` 中没有对应记录，CLI 直接返回 `task "<submit_id>" not found`，不会进入完整远端查询流程。
- `~/.dreamina_cli/tasks.db` 的核心表结构为：

  ```sql
  CREATE TABLE `aigc_task` (
    `submit_id` text,
    `gen_task_type` text NOT NULL,
    `uid` integer NOT NULL,
    `create_time` integer NOT NULL,
    `update_time` integer NOT NULL,
    `logid` text NOT NULL DEFAULT "",
    `request` text NOT NULL,
    `gen_status` text NOT NULL,
    `fail_reason` text NOT NULL DEFAULT "",
    `result_json` text NOT NULL DEFAULT "",
    `commerce_info` text NOT NULL DEFAULT "",
    PRIMARY KEY (`submit_id`)
  );
  ```

判断：

- CLI 的 `query_result` 不是一个纯远端查询命令，它绑定了本地任务库。
- 后端不能把 CLI `query_result` 作为长期主路径，否则会引入第二套隐藏任务状态源。
- 后端应继续使用 `model.Task.PrivateData.UpstreamTaskID` 保存 Dreamina 的真实 `submit_id`，并由 `dreamina_oauth` adaptor 自己查询远端。

### 验证 3：workspace/session 路径

已确认：

- `dreamina session list -n 1` 返回成功，且输出是表格列：`ID`、`NAME`、`PINNED`、`UPDATED_AT`。
- `dreamina session list --help` 明确说明 session `0` 是默认会话，不可重命名、不可删除。
- CLI 二进制中存在 workspace API 路径：

  - `/mweb/v1/workspace/list`
  - `/mweb/v1/workspace/create`
  - `/mweb/v1/workspace/update`
  - `/mweb/v1/workspace/delete`

判断：

- CLI 的 session 概念基本对应远端 workspace。
- 第一版后端无需暴露完整 session 管理；Canvas 侧继续使用 `metadata.jimeng.canvas_project_id`、`metadata.jimeng.canvas_node_id` 表达项目归属更稳。
- 如后续必须映射 Dreamina workspace，可将 `session=0` 作为默认 workspace 策略，再补充专门的 workspace 绑定表。

### 验证 4：上传/资源链路线索

已确认：

- 对带本地图片的命令加本地代理并立即返回 502，`dreamina image_upscale --image=/tmp/codex-dreamina-1x1.png --resolution_type=2k --poll=0` 会尝试连接：

  - `bytetsd-router.byted.org:443`
  - `jimeng.jianying.com:443`

- `dreamina multimodal2video --image=/tmp/codex-dreamina-1x1.png --prompt=validation --duration=4 --poll=0` 也会尝试连接同样域名。
- CLI 二进制中存在资源上传路径和结构：

  - `/mweb/v1/get_upload_token`
  - `/dreamina/mcp/v1/resource_store`
  - `ResourceUpload.getUploadToken`
  - `ResourceUpload.resourceStore`
  - `ResourceUpload.uploadVideoAudio`
  - `UploadAuth`
  - `resource_id`
  - `image_resource_id_list`
  - `video_resource_id_list`
  - `audio_resource_id_list`
  - `resource_id_reuse_type`

判断：

- 图片、视频、音频输入不是直接塞进生成请求体，而是先经过上传/资源登记流程，生成 `resource_id` 或等价资源引用。
- 上传是当前 HTTP 适配器最大的未知项，需要优先复刻 `get_upload_token -> object upload -> resource_store -> resource_id` 这一段。

### 验证 5：生成提交路径线索

已确认 CLI 二进制中存在：

- `/dreamina/cli/v1/image_generate`
- `/dreamina/cli/v1/video_generate`
- `/dreamina/mcp`
- `/dreamina/cli/v1/dreamina_cli_user_info`
- `/mweb/v1/get_history_by_ids`
- `[SubmitTask] submit generation task finished gen_task_type=%s submit_id=%s task_status=%s remote_logid=%s input=%s`
- `[QueryResult] fallback to querying for unrecognized status=%d submit_id=%s`
- `AigcComplianceConfirmationRequired`

判断：

- 生图和生视频很可能分别走 `/dreamina/cli/v1/image_generate`、`/dreamina/cli/v1/video_generate`，共用 MCP/authorized post 客户端封装。
- 结果查询可能不是单独 `query_result` 路径，而可能复用 `/mweb/v1/get_history_by_ids` 或 MCP history/task 查询封装。仍需用真实历史任务或更深的动态观测确认。
- 合规确认不是偶发文案，而是 CLI 内置错误分支；后端需要把它映射为明确的用户可处理错误。

### 验证 6：TLS 明文抓取尝试

尝试过本地临时 MITM 代理，目标是只抓方法、路径、Host、请求体字段名，并脱敏 Authorization/Cookie/token。

结果：

- CLI 确实会通过代理发出 `CONNECT bytetsd-router.byted.org:443`、`CONNECT jimeng.jianying.com:443`。
- macOS/Go 证书校验拒绝临时 CA，TLS 握手失败，未能获取 HTTPS 明文请求。
- 未把临时 CA 写入系统信任链，避免污染用户机器安全状态。

判断：

- 后续若要动态抓明文，建议使用更正式的可撤销方式，例如单独测试机、容器、系统临时信任配置，或在用户明确允许后短时安装并移除本地 CA。
- 当前不应为了抓包强行修改系统证书信任。

### 验证 7：隔离 HOME + 假任务库尝试触发远端 `query_result`

做法：

- 创建临时 HOME。
- 在临时 HOME 下创建独立 `.dreamina_cli/tasks.db`。
- 插入假 `aigc_task` 记录，避免污染真实用户的 `~/.dreamina_cli/tasks.db`。
- 使用假 `submit_id` 调用 `dreamina query_result`。

结果：

- CLI 不再输出 `task "<submit_id>" not found`，说明已绕过“本地没有任务记录”的第一道门槛。
- 但 CLI 退出码为 `1`，无 stdout/stderr，临时日志为空。
- 尝试过 `gen_task_type=text2image`、`image_generate`、`text2video` 等假类型，均未形成可观测远端查询。

判断：

- `query_result` 还依赖本地任务记录中的 `gen_task_type`、`request`、`gen_status` 等字段满足 CLI 内部枚举和结构要求。
- 没有真实历史任务记录时，伪造 SQLite 行不足以可靠触发远端查询。
- 若要拿到远端查询协议，最小路径是：

  1. 找到一个真实历史任务的 `submit_id` 和本地 `aigc_task` 行。
  2. 或执行一次明确授权的最低成本生成，得到真实 `submit_id`。
  3. 再对真实 `submit_id` 做 `query_result` 动态观测。

### 验证 8：当前历史任务状态

已执行：

```bash
dreamina list_task --limit=5
```

结果：

```json
[]
```

判断：

- 当前本机 CLI 任务库没有可复用的真实历史任务。
- 暂时无法在“不生成新任务、不消耗积分”的前提下自然触发真实 `query_result` 远端查询。

### 验证 9：请求/响应结构线索

通过静态二进制字符串可见：

- `mcp.Text2ImageRequest`
- `mcp.Image2ImageRequest`
- `mcp.Text2VideoRequest`
- `mcp.Image2VideoRequest`
- `mcp.Frames2VideoRequest`
- `mcp.MultiModal2VideoRequest`
- `mcp.Ref2VideoRequest`
- `mcp.UpscaleRequest`
- `mcp.HistoryTask`
- `mcp.HistoryImageInfo`
- `mcp_api.GenerateToolResp`
- `mcp_api.GenerateToolData`
- `mcp_api.SubmitInfo`

`GenerateToolData` 暴露出的响应字段 getter 包括：

- `GetContents`
- `GetLlmContents`
- `GetHistoryID`
- `GetCommerceInfo`
- `GetSubmitInfo`
- `GetSubmitID`
- `GetResultCode`
- `GetMetrics`
- `GetRatio`
- `GetPreGenItemIds`
- `GetForecastResolution`
- `GetItemIDList`
- `GetResourceType`
- `GetEnd`
- `GetModelKey`

判断：

- Dreamina 生成提交响应中，`submit_id` 明确属于 `GenerateToolData`。
- 计费/权益信息可能在 `CommerceInfo` 或 `SubmitInfo` 中。
- 结果内容可能在 `Contents`、`LlmContents`、`ItemIDList`、`HistoryID` 等字段中，需要结合真实响应确认。

## 当前验证边界

在不消耗积分、不修改系统证书信任、不读取真实凭证值的前提下，目前已经确认：

- 登录态形态：OAuth Device Flow + macOS Keychain/keyring。
- workspace/session：远端 workspace API 存在，`session=0` 是默认会话。
- 上传链路：存在 `get_upload_token`、object upload、`resource_store`、`resource_id` 流程。
- 生成链路：存在 `/dreamina/cli/v1/image_generate`、`/dreamina/cli/v1/video_generate`。
- 查询链路：CLI 的 `query_result` 依赖本地 `tasks.db`，不能直接当纯远端查询工具。

仍未确认：

- 授权头的确切形式。
- `image_generate` / `video_generate` 的完整请求体。
- `get_history_by_ids` 或远端查询接口的完整请求体和响应结构。
- `get_upload_token` 的请求体、返回体，以及对象存储上传细节。

## 下一步最小可执行方案

### 方案 A：真实历史任务优先，零新增消耗

前提：

- 本机或其他机器上已有一个 Dreamina CLI 生成过的真实任务。

动作：

- 复制或导出该任务的 `submit_id` 和对应 `aigc_task` 行的脱敏结构。
- 只运行 `dreamina query_result --submit_id=<真实历史 submit_id>`。
- 观察远端查询路径和响应结构。

优点：

- 不新增生成消耗。
- 能直接验证查询和结果转换。

缺点：

- 当前本机 `list_task --limit=5` 为空，暂时没有可用历史任务。

### 方案 B：一次最低成本生成，换取完整链路样本

前提：

- 用户明确允许执行一次会消耗积分的最低成本生成。

建议命令：

```bash
dreamina text2image --prompt="一只小猫，白底，测试用" --ratio=1:1 --resolution_type=2k --generate_num=1 --poll=0
```

动作：

- 保存返回的 `submit_id`。
- 查看本地 `aigc_task` 行结构，但脱敏 prompt、uid、logid。
- 运行 `query_result` 获取真实查询流程。
- 若允许正式抓包，再用可撤销 CA/专用测试机观察 HTTPS 明文。

优点：

- 最快拿到真实 `submit_id`、本地任务记录、查询路径、响应结构。

缺点：

- 会消耗 Dreamina 积分。

### 方案 C：先做后端接口骨架，不等完整私有协议

动作：

- 在后端先设计 `dreamina_oauth` adaptor 的接口边界。
- 暂不实现真实 HTTP 上游，只定义：

  - credential provider
  - workspace resolver
  - resource uploader
  - generation submitter
  - result fetcher
  - Dreamina error mapper

优点：

- 可以先把 new-api 内部落点、DTO、计费、任务模型接好。
- 等协议细节确认后，只替换 adaptor 内部实现。

缺点：

- 不能立即跑通真实生成。
