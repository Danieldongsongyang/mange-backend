# 即梦 CLI 接入当前后端方案

本文档依据 `/Users/a1/Documents/即梦 CLI 体验指南.md` 整理，目标是说明如何把即梦 CLI 的生成能力接入当前后端项目，并让“漫歌”无限画布前端的相关请求统一交由本项目处理。

## 结论

建议不要改动现有 `relay/channel/task/jimeng` 的官方 HTTP API 适配器。当前项目已经有一条即梦官方 API 链路，走的是火山/即梦 HTTP 接口、签名、`TaskAdaptor`、任务轮询和计费流程；即梦 CLI 是另一种本地命令行执行能力，依赖服务器机器上的 `dreamina` 可执行文件、登录态和本地任务数据库。

推荐新增一条独立的 `jimeng_cli` 业务能力：

- 漫歌前端只请求当前后端，不直接调用即梦 CLI。
- 当前后端负责鉴权、参数校验、上传文件落地、调用 `dreamina`、保存任务、查询结果、下载或代理产物。
- 即梦 CLI 的登录态和凭证只保存在服务器运行用户的 `~/.dreamina_cli` 下，不暴露给前端。
- 一期优先做业务接口，服务漫歌画布；二期如需统一到 `/v1/video/generations` 或渠道管理，再抽象成 `relay.TaskAdaptor` 或新增渠道类型。

## 即梦 CLI 能力摘要

即梦 CLI 安装和更新：

```bash
curl -fsSL https://jimeng.jianying.com/cli | bash
```

登录和自检：

```bash
dreamina login
dreamina login --debug
dreamina user_credit
```

核心生成命令：

```bash
dreamina text2image --prompt="一只戴墨镜的橘猫" --ratio=1:1 --resolution_type=2k --poll=30
dreamina text2video --prompt="镜头推进，一只橘猫从沙发上跳下来" --duration=5 --ratio=16:9 --video_resolution=720P --poll=30
dreamina image2image --images ./input.png --prompt="改成水彩风格" --resolution_type=2k --poll=30
dreamina image2video --image ./first_frame.png --prompt="镜头慢慢推近" --duration=5 --poll=30
dreamina query_result --submit_id=<submit_id>
dreamina query_result --submit_id=<submit_id> --download_dir=./downloads
dreamina list_task
```

CLI 本地文件：

- `~/.dreamina_cli/config.toml`：请求环境配置。
- `~/.dreamina_cli/tasks.db`：CLI 本地任务记录。
- `~/.dreamina_cli/logs/`：CLI 运行日志。

注意：`image2image` 的 `--images` 和 `image2video` 的 `--image` 需要本地图片路径，因此后端必须先接收前端上传的图片并保存为服务器本地文件，再把路径传给 CLI。

## 与当前项目的关系

当前项目已经具备这些可复用能力：

- `router/api-router.go`：业务 API 路由，适合放漫歌专用接口。
- `router/video-router.go`：已有视频任务路由和即梦官方格式兼容路由。
- `controller/task.go`：任务列表查询。
- `model/task.go`：通用异步任务表，可保存平台、动作、状态、进度、结果、失败原因、计费额度等。
- `service/task_polling.go`：已有任务轮询框架。
- `relay/channel/task/jimeng/adaptor.go`：即梦官方 HTTP API 适配器，不建议混入 CLI 逻辑。
- `common/json.go`：项目规定的 JSON 编解码入口，新代码必须使用 `common.Marshal`、`common.Unmarshal` 等包装函数。

建议新增的逻辑位置：

```text
dto/canvas_generation.go              # 漫歌/画布生成请求与响应 DTO
pkg/dreamina/cli.go                   # dreamina 命令封装，只负责执行 CLI 和解析输出
pkg/dreamina/result.go                # CLI 输出结构、状态抽取、结果 URL/文件抽取
service/canvas_generation.go          # 业务编排：鉴权后的生成、查询、下载、任务落库
controller/canvas_generation.go       # Gin handler
router/api-router.go                  # 注册 /api/canvas/generations 路由
```

如果后续希望让它进入统一模型渠道体系，再新增：

```text
relay/channel/task/jimengcli/adaptor.go
constant/channel.go                   # 新增 ChannelTypeJimengCLI
constant/api_type.go                  # 新增 APITypeJimengCLI
relay/relay_adaptor.go                # 注册 task adaptor
web/default/...                       # 管理后台新增渠道类型文案和配置
```

## 推荐接口设计

一期建议提供面向漫歌的业务接口，而不是直接让前端拼即梦 CLI 参数。

### 提交生成任务

```http
POST /api/canvas/generations
Authorization: Bearer <token> 或使用现有登录态
Content-Type: application/json 或 multipart/form-data
```

JSON 请求示例：

```json
{
  "type": "text2image",
  "prompt": "一只戴墨镜的橘猫",
  "ratio": "1:1",
  "resolution_type": "2k",
  "poll_seconds": 0,
  "canvas_id": "canvas_xxx",
  "node_id": "node_xxx"
}
```

图生图建议使用 `multipart/form-data`，图片由后端保存成本地临时文件：

```text
type=image2image
prompt=改成水彩风格
resolution_type=2k
image=<file>
canvas_id=canvas_xxx
node_id=node_xxx
```

响应示例：

```json
{
  "success": true,
  "data": {
    "task_id": "task_xxx",
    "submit_id": "dreamina_submit_id",
    "status": "submitted",
    "progress": "0%",
    "result_urls": [],
    "raw": {}
  }
}
```

### 查询任务

```http
GET /api/canvas/generations/:task_id
```

响应示例：

```json
{
  "success": true,
  "data": {
    "task_id": "task_xxx",
    "submit_id": "dreamina_submit_id",
    "status": "succeeded",
    "progress": "100%",
    "result_urls": [
      "/api/canvas/assets/task_xxx/output.png"
    ],
    "fail_reason": "",
    "raw": {}
  }
}
```

### 查询当前用户任务列表

可以优先复用现有接口：

```http
GET /api/task/self?platform=jimeng_cli
```

如漫歌需要按画布筛选，再新增：

```http
GET /api/canvas/generations?canvas_id=canvas_xxx
```

## 请求类型与 CLI 命令映射

后端收到业务请求后，按 `type` 映射到 CLI 子命令。执行命令时必须使用 `exec.CommandContext(ctx, bin, args...)` 的参数数组形式，不要拼接 shell 字符串。

| 请求类型 | CLI 命令 | 必填字段 | 可选字段 |
| --- | --- | --- | --- |
| `text2image` | `dreamina text2image` | `prompt` | `ratio`, `resolution_type`, `poll_seconds` |
| `image2image` | `dreamina image2image` | `prompt`, 本地图片路径 | `resolution_type`, `poll_seconds` |
| `text2video` | `dreamina text2video` | `prompt` | `duration`, `ratio`, `video_resolution`, `poll_seconds` |
| `image2video` | `dreamina image2video` | `prompt`, 本地图片路径 | `duration`, `ratio`, `poll_seconds` |

参数转换示例：

```go
args := []string{
    "text2image",
    "--prompt", req.Prompt,
    "--ratio", req.Ratio,
    "--resolution_type", req.ResolutionType,
}
if req.PollSeconds > 0 {
    args = append(args, "--poll", strconv.Itoa(req.PollSeconds))
}
cmd := exec.CommandContext(ctx, dreaminaBin, args...)
```

建议默认不传 `--poll`，让接口快速返回 `submit_id`，再由后端轮询或前端查询。如果要为了交互体验短等待，`poll_seconds` 应限制在较小范围，例如 `0` 到 `30` 秒。

## 任务模型

一期可以复用 `model.Task`，避免新增跨数据库迁移：

```text
Platform   = "jimeng_cli"
Action     = "text2image" / "image2image" / "text2video" / "image2video"
TaskID     = model.GenerateTaskID() 生成的对外 ID
PrivateData.UpstreamTaskID = dreamina submit_id
Status     = NOT_START / SUBMITTED / IN_PROGRESS / SUCCESS / FAILURE
Progress   = 0% / 10% / 30% / 100%
Data       = CLI 原始 JSON 输出或规范化结果
FailReason = 失败原因
Properties.Input = prompt 摘要或画布节点信息
```

`PrivateData` 当前已有 `UpstreamTaskID` 和 `ResultURL`，可以直接保存：

- `UpstreamTaskID`：即梦 CLI 返回的 `submit_id`。
- `ResultURL`：成功后可访问的结果 URL，或当前后端下载后的资源 URL。

如果需要保存 `canvas_id`、`node_id`、输入图片文件 ID 等画布上下文，优先放在 `Data` 或新增轻量业务表。新增表时必须使用 GORM，并确保 SQLite、MySQL、PostgreSQL 兼容。

## 状态映射

即梦 CLI 文档只明确说明超时时会返回 `querying` 状态，最终结果通过 `query_result` 查询。实现时建议做宽松映射，兼容 CLI 后续字段变化。

| CLI 状态 | 内部状态 | 说明 |
| --- | --- | --- |
| `querying` | `SUBMITTED` 或 `IN_PROGRESS` | 已提交但未完成 |
| `running` / `processing` | `IN_PROGRESS` | 处理中 |
| `success` / `succeeded` / `done` | `SUCCESS` | 成功 |
| `failed` / `failure` / `error` | `FAILURE` | 失败 |
| 未识别但有 `submit_id` | `SUBMITTED` | 保守认为已提交 |
| 未识别且无 `submit_id` | `FAILURE` | 视为提交失败 |

结果抽取也要宽松处理：

- 从 `submit_id`、`task_id`、`id` 中尝试提取上游任务 ID。
- 从 `url`、`urls`、`image_urls`、`video_url`、`result_url`、`downloaded_files` 等字段中尝试提取结果。
- 原始 CLI 输出完整保存到 `Task.Data`，方便排查。

## 文件处理

输入文件：

- 前端上传的图片由当前后端保存到受控目录，例如 `data/dreamina/uploads/`。
- 文件名使用随机 ID，不使用用户原始文件名作为路径。
- 校验 MIME、扩展名和大小，避免把任意文件传给 CLI。
- 传给 CLI 的必须是服务器本地路径。

输出文件：

- 查询成功时可以使用：

```bash
dreamina query_result --submit_id=<submit_id> --download_dir=<download_dir>
```

- 下载目录建议为 `data/dreamina/results/<task_id>/`。
- 对前端只返回后端 URL，不返回服务器本地路径。
- 可以新增资源访问接口，例如：

```http
GET /api/canvas/assets/:task_id/:filename
```

如果即梦返回的是远程 URL，也建议保存到 `PrivateData.ResultURL`，前端可直接使用或通过当前后端代理访问。

## CLI 封装层设计

建议在 `pkg/dreamina` 中只处理本地命令执行，不掺入用户、计费、任务表逻辑。

核心接口：

```go
type Client struct {
    Bin         string
    WorkDir     string
    DownloadDir string
    Timeout     time.Duration
}

type SubmitRequest struct {
    Type            string
    Prompt          string
    ImagePaths      []string
    Ratio           string
    ResolutionType  string
    Duration        int
    VideoResolution string
    PollSeconds     int
}

type Result struct {
    SubmitID   string
    Status     string
    ResultURLs []string
    Files      []string
    Raw        map[string]any
}

func (c *Client) Submit(ctx context.Context, req SubmitRequest) (*Result, error)
func (c *Client) Query(ctx context.Context, submitID string, downloadDir string) (*Result, error)
func (c *Client) UserCredit(ctx context.Context) (map[string]any, error)
```

实现要求：

- 用 `exec.CommandContext`，不用 shell。
- stdout/stderr 都要采集；失败时返回 stderr 摘要。
- JSON 解析使用 `common.Unmarshal`。
- JSON 输出再返回给调用层时使用 `common.Marshal`。
- 命令超时后 kill 进程，并把任务标记为可稍后查询或失败。
- 日志中不要打印完整登录态、凭证、本地敏感路径。

## 业务服务层设计

`service/canvas_generation.go` 负责业务编排：

1. 校验用户身份和请求参数。
2. 保存上传文件到本地受控目录。
3. 创建 `model.Task`，状态为 `NOT_START`。
4. 调用 `pkg/dreamina.Client.Submit`。
5. 如果拿到 `submit_id`，写入 `PrivateData.UpstreamTaskID`，状态改为 `SUBMITTED` 或 `IN_PROGRESS`。
6. 如果 CLI 直接返回成功结果，写入 `Data`、`PrivateData.ResultURL`，状态改为 `SUCCESS`。
7. 如果失败，写入 `FailReason`，状态改为 `FAILURE`，必要时退款。
8. 查询时调用 `Client.Query`，更新任务状态和结果。

计费可以分阶段做：

- 一期：按固定模型价格或固定额度预扣费；失败时退款。
- 二期：按类型、分辨率、时长增加倍率，例如 `text2video` 按 `duration` 加倍率。
- 如果接入项目的表达式计费系统，必须先阅读 `pkg/billingexpr/expr.md`。

## 后台轮询

可选两种方案：

### 方案 A：前端主动查询

提交后返回 `task_id`，漫歌前端定时请求：

```http
GET /api/canvas/generations/:task_id
```

后端在每次查询时调用 `dreamina query_result` 更新状态。这个方案简单，适合一期。

### 方案 B：接入现有任务轮询

在 `service/task_polling.go` 的分发逻辑中新增 `jimeng_cli`：

```go
case constant.TaskPlatformJimengCLI:
    _ = UpdateDreaminaCLITasks(context.Background(), taskChannelM, taskM)
```

`UpdateDreaminaCLITasks` 内部调用 `pkg/dreamina.Client.Query`。这个方案能让任务即使前端不打开也持续推进，适合二期或生产化。

## 配置项

建议先使用环境变量，后续再进入后台配置：

```text
DREAMINA_CLI_ENABLED=true
DREAMINA_CLI_BIN=dreamina
DREAMINA_CLI_WORKDIR=data/dreamina
DREAMINA_CLI_UPLOAD_DIR=data/dreamina/uploads
DREAMINA_CLI_DOWNLOAD_DIR=data/dreamina/results
DREAMINA_CLI_TIMEOUT_SECONDS=120
DREAMINA_CLI_MAX_POLL_SECONDS=30
DREAMINA_CLI_MAX_UPLOAD_MB=20
```

服务启动或管理员自检时执行：

```bash
dreamina user_credit
```

如果失败，应提示管理员先在运行后端的系统用户下完成：

```bash
dreamina login
```

Docker 部署时需要特别注意：

- 容器内必须安装 `dreamina`。
- 后端进程运行用户的 home 目录需要有 `~/.dreamina_cli/config.toml` 和登录态。
- `~/.dreamina_cli`、上传目录、结果目录建议挂载为持久化 volume。

## 安全要求

- 不允许前端传入任意 CLI 参数，只接受白名单字段。
- 不允许把用户输入拼成 shell 命令。
- 不返回服务器本地路径、CLI 日志路径、登录态路径。
- 上传文件保存目录必须由后端生成，禁止路径穿越。
- 限制 `prompt` 长度、图片大小、轮询时长和并发数。
- 对生成接口加现有用户鉴权和速率限制。
- 失败响应只返回必要错误信息，详细 stdout/stderr 写内部日志。
- 即梦 CLI 使用的是服务器账号，不是前端用户个人账号；产品上要明确额度归属。

## 与漫歌无限画布的协作边界

漫歌前端只负责画布交互和发起请求：

- 文本提示词、选区、参考图、节点 ID、画布 ID 从前端传给当前后端。
- 当前后端负责提交生成、查询、存储结果、返回可访问 URL。
- 前端拿到结果 URL 后，把图片或视频插入画布节点。
- 漫歌项目内不保存即梦账号、不执行 `dreamina`、不直接访问上游生成服务。

推荐前端保存的最小生成元数据：

```json
{
  "backend_task_id": "task_xxx",
  "generation_type": "text2image",
  "prompt": "一只戴墨镜的橘猫",
  "status": "succeeded",
  "result_url": "/api/canvas/assets/task_xxx/output.png"
}
```

## 实施顺序

1. 在服务器上安装 `dreamina`，完成 `dreamina login`，用 `dreamina user_credit` 自检。
2. 新增 `pkg/dreamina`，实现 CLI 执行、JSON 解析、状态和结果抽取。
3. 新增 `dto/canvas_generation.go`，定义提交和查询响应。
4. 新增 `service/canvas_generation.go`，实现文件保存、任务落库、提交、查询、结果更新。
5. 新增 `controller/canvas_generation.go` 和 `/api/canvas/generations` 路由。
6. 先采用前端主动查询方案，跑通 `text2image` 和 `image2image`。
7. 再扩展 `text2video` 和 `image2video`。
8. 根据实际 CLI 输出补齐结果字段解析。
9. 加单元测试：参数到命令 args 的映射、状态映射、结果抽取、失败解析。
10. 加集成测试或手工验收：提交任务、查询中间态、成功下载、失败退款。

## 验收标准

- 后端能通过 `dreamina user_credit` 自检 CLI 可用。
- 漫歌前端能通过当前后端提交文生图请求，并拿到 `task_id`。
- 前端查询 `task_id` 能拿到处理中、成功或失败状态。
- 成功任务返回可访问的图片或视频 URL。
- 图生图和图生视频不会把前端上传文件路径直接透传给 CLI，而是使用后端保存后的本地路径。
- 失败任务能记录失败原因，且不会泄露服务器本地敏感路径。
- 新代码遵守项目 JSON 规则，业务代码不直接调用 `encoding/json` 的 Marshal/Unmarshal。
- 不破坏现有即梦官方 API 路由和 `relay/channel/task/jimeng` 行为。
