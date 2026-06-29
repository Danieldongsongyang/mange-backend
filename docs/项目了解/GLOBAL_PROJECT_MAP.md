# 项目全局理解地图

这份文档记录一次对子代理全局探究结果的沉淀。目标不是解决某一个具体需求，而是给后续任何开发任务提供通用上下文：先知道项目怎么运转、请求怎么流动、常见改动应该去哪找、哪些约束不能踩。

## 一句话心智模型

这是一个 Go 后端驱动的 AI API 网关。它把用户侧的 OpenAI / Claude / Gemini / 图片 / 音频 / rerank / 异步任务等入口统一接入，再根据模型、分组、渠道配置、计费规则和 provider adaptor 转发到不同上游。

核心链路可以记成：

```text
router -> middleware -> controller -> relay/service -> channel adaptor -> upstream provider
                                      -> response handler -> billing/log/model
```

后台管理、用户、渠道、令牌、日志等普通业务接口也在同一个 Gin 服务里，但它们主要走：

```text
router -> controller -> service -> model
```

## 关键目录职责

### `main.go`

进程启动入口。负责初始化配置、数据库、Redis、i18n、缓存、后台任务、Gin server，并嵌入前端构建产物。

后续如果需求是“启动时加载某个全局资源”“增加后台定时任务”“调整静态前端托管”，先看这里和它调用的初始化函数。

### `router/`

HTTP 路由层。它不做复杂业务，主要负责把不同路径挂到 controller。

重点文件：

- `router/main.go`：总路由装配入口。
- `router/api-router.go`：后台 API 主地图，比如用户、渠道、token、日志、模型、设置。
- `router/relay-router.go`：AI API 兼容入口，比如 `/v1/chat/completions`、`/v1/messages`、`/v1/responses`、Gemini native、图片、音频、rerank。
- `router/video-router.go`：视频和任务类入口。
- `router/web-router.go`：默认/经典前端主题和 SPA fallback。

判断经验：如果是“新增一个 HTTP 路径”，先从 `router/` 开始；如果只是改变已有接口行为，通常不改 router。

### `middleware/`

请求进入 controller 前的横切逻辑。最重要的是渠道分发。

重点文件：

- `middleware/distributor.go`：根据模型、用户分组、token 限制、渠道可用性、优先级、权重等选择具体 channel，并把 channel 信息写入 Gin context。

这个文件是理解“为什么请求最终走某个渠道”的关键。很多 relay 后续逻辑不是直接查数据库，而是从 context 读取 middleware 写进去的 channel 元信息。

### `controller/`

请求处理入口和业务编排层。它通常负责解析参数、校验权限、调用 service/model、组织响应。

重点文件：

- `controller/relay.go`：AI relay 的总入口。负责请求校验、生成 `RelayInfo`、敏感词检查、预估 token、预扣费、重试和错误处理。
- `controller/channel.go`：后台渠道管理。新增/编辑/测试/复制/获取模型等渠道操作集中在这里。
- `controller/channel-test.go`：渠道测试链路。
- `controller/model.go`：模型列表、模型 owner、dashboard 模型数据。
- `controller/task_video.go`、`controller/video_proxy_gemini.go`：视频/任务类辅助入口。

判断经验：如果需求是“接口返回什么”“后台按钮调用后发生什么”“一次请求的业务流程怎么编排”，优先看 `controller/`。

### `relay/`

AI API 转发的协议层和模式层。它把 controller 已经解析好的请求交给具体 handler，再由 handler 调用 channel adaptor。

重点文件：

- `relay/compatible_handler.go`：OpenAI-compatible 文本主链路。
- `relay/claude_handler.go`：Claude 格式入口。
- `relay/gemini_handler.go`：Gemini 格式入口。
- `relay/responses_handler.go`：OpenAI Responses API。
- `relay/embedding_handler.go`：embedding。
- `relay/image_handler.go`：图片生成/编辑。
- `relay/audio_handler.go`：音频。
- `relay/rerank_handler.go`：rerank。
- `relay/websocket.go`：realtime/websocket。
- `relay/relay_adaptor.go`：`APIType -> channel.Adaptor` 的注册表，也是 task adaptor 注册点。

判断经验：如果需求是“某类 AI 请求如何转换、转发、处理响应”，先看 `relay/*_handler.go`，然后追到 `relay/channel/<provider>`。

### `relay/common/`

relay 过程里的共享上下文和工具。

重点文件：

- `relay/common/relay_info.go`：`RelayInfo` 和 `ChannelMeta`。这是一次 relay 请求的运行时事实表。
- `relay/common/override.go`：渠道参数覆盖和 header override 逻辑。
- `relay/common/relay_utils.go`：构建上游 URL、API version 等小工具。

`RelayInfo.InitChannelMeta` 会从 Gin context 中读取 middleware 写入的渠道信息，并计算 `ApiType`。后续 adaptor、计费、日志都会依赖它。

### `relay/channel/`

上游 provider 适配层。每个目录通常对应一个 provider 或一类协议适配。

重点文件：

- `relay/channel/adapter.go`：普通 channel adaptor 和 task adaptor 的接口契约。
- `relay/channel/api_request.go`：通用 HTTP/WebSocket 请求发送逻辑、header override 应用逻辑。
- `relay/channel/openai/`：OpenAI 兼容基座，同时包含 Azure、OpenRouter、Xinference 等历史兼容分支。
- `relay/channel/claude/`：Anthropic/Claude 原生处理。
- `relay/channel/gemini/`、`relay/channel/vertex/`：Gemini/Vertex 相关处理。
- `relay/channel/task/`：异步任务/视频类 provider 适配。

判断经验：如果需求与某个上游 provider 的 URL、鉴权、请求字段、响应解析、usage 计算相关，主要改这里。

### `service/`

业务逻辑层。它承接 controller/relay 中可复用、跨模块的行为。

常见职责：

- token 估算、计费、扣费、结算。
- 格式转换，如 Claude/Gemini/OpenAI 之间的请求转换。
- 渠道亲和、分组、工具计费。
- 异步任务轮询。
- OAuth、凭证刷新、特殊 provider 辅助流程。

判断经验：如果逻辑不属于 HTTP 入参出参，也不属于数据库模型本身，而是可复用业务规则，通常放在 `service/`。

### `model/`

GORM 模型和数据库访问层。数据库要同时兼容 SQLite、MySQL、PostgreSQL。

重点对象：

- `model.Channel`：渠道配置，包括 type、base URL、key、模型映射、状态码映射、设置项等。
- `model.Ability`：模型可用性和渠道能力。
- 用户、token、日志、任务、价格等持久化模型。

判断经验：如果需求涉及表结构、查询、更新、缓存 DB 结果，先看 `model/`。写数据库逻辑时尽量用 GORM 抽象，谨慎写 raw SQL。

### `constant/`

全局常量和枚举。

重点文件：

- `constant/channel.go`：`ChannelType`、默认 base URL、渠道展示名。
- `constant/api_type.go`：`APIType`，用于选择具体 adaptor。
- `constant/context_key.go`：Gin context key。
- `constant/endpoint_type.go` 或相关 endpoint 常量：模型支持的 endpoint 类型。

这里的数字常量很多已经被数据库和前端引用。不要随意重排已有值。

### `common/`

全项目共享基础工具。

重点文件：

- `common/api_type.go`：`ChannelType -> APIType` 映射。
- `common/json.go`：项目 JSON wrapper。
- `common/endpoint_type.go`：根据渠道和模型判断优先 endpoint 类型。
- 还有环境变量、Redis、加密、日志、Gin context 辅助等。

特别约束：业务代码不要直接调用 `encoding/json` 的 marshal/unmarshal，使用 `common.Marshal`、`common.Unmarshal`、`common.DecodeJson` 等 wrapper。

### `dto/`

请求/响应结构体。relay 请求 DTO、provider 响应 DTO、后台接口 DTO 都可能在这里。

重要约束：上游 relay 请求中，可选标量字段要用指针类型加 `omitempty`，避免显式传入 `0`、`0.0`、`false` 时被误删。

### `setting/`

全局运行时设置。包括模型设置、倍率/价格设置、系统设置、性能设置、操作设置等。

如果需求是“后台配置影响运行时行为”，经常会同时涉及：

```text
web/default -> controller/option or settings controller -> setting/* -> service/relay
```

### `web/default/`

默认前端，React 19 + TypeScript + Rsbuild + Base UI + Tailwind。渠道管理、模型管理、系统设置等后台页面都在这里。

对于后端二创来说，常见需要知道：

- 后台渠道页面配置集中在 `web/default/src/features/channels/`。
- 模型/价格设置集中在 `web/default/src/features/system-settings/models/` 和 `web/default/src/features/pricing/`。
- i18n 文件在 `web/default/src/i18n/locales/{lang}.json`。
- 前端优先用 `bun`，例如 `bun run build`。

### `web/classic/`

旧版前端主题，React 18 + Vite + Semi Design。很多渠道枚举、图标和后台表单也有一份 classic 实现。如果功能需要两个主题都可用，不能只改 `web/default`。

## Relay 请求主链路

以 `/v1/chat/completions` 为例：

1. `router/relay-router.go` 把路径挂到 `controller.Relay`。
2. `controller/relay.go` 解析请求体，判断 relay format 和 relay mode。
3. middleware 分发器已经选好 channel，并把 channel type、key、base URL、setting、override 等写入 Gin context。
4. `relay/common/relay_info.go` 生成 `RelayInfo`，并通过 `common.ChannelType2APIType` 得到 `ApiType`。
5. `relay/compatible_handler.go` 调 `relay.GetAdaptor(info.ApiType)`。
6. adaptor 执行 `Init`、`ConvertOpenAIRequest` 等转换。
7. handler 用 `common.Marshal` 序列化上游请求体，并应用 param override。
8. adaptor 的 `DoRequest` 通常调用 `channel.DoApiRequest`。
9. `channel.DoApiRequest` 调 adaptor 的 `GetRequestURL` 和 `SetupRequestHeader`，再应用 header override，最后发给上游。
10. adaptor 的 `DoResponse` 处理响应，通常复用 OpenAI/Claude/Gemini handler，也可能自定义解析。
11. controller/relay 根据 usage 做结算、日志、错误处理和重试判断。

这个链路是理解绝大多数 AI 请求行为的主线。

## 普通后台 API 主链路

后台管理类接口通常更接近经典分层：

```text
router/api-router.go
  -> controller/<domain>.go
    -> service/<domain>.go
      -> model/<domain>.go
```

不是所有接口都有完整 service 层。有些 controller 会直接调用 model，这在当前代码里是既有风格。新增代码时优先观察相邻实现，不要为了“标准分层”强行搬动大量旧逻辑。

## 异步任务和视频主链路

异步任务和视频类 provider 与普通 chat completion 不同，常见流程是：

```text
submit request -> 创建/保存 task -> 返回 task id -> 后台轮询 provider -> 更新 task 状态 -> 用户 fetch 结果
```

相关位置：

- `relay/relay_task.go`
- `relay/channel/task/`
- `service/task_polling.go`
- `model/task.go`
- `controller/task_video.go`
- `router/video-router.go`

如果需求是视频生成、任务查询、任务结果转换、轮询结算，优先从这些文件看。

## 渠道和模型的核心概念

### ChannelType

用户和后台配置看到的渠道类型。定义在 `constant/channel.go`。它对应数据库里 channel 的 `type`。

### APIType

relay 内部用来选择 adaptor 的类型。定义在 `constant/api_type.go`。

很多 `ChannelType` 可以复用同一个 `APIType`。例如某些 OpenAI-compatible 渠道可以复用 OpenAI adaptor；也有渠道拥有独立 adaptor。

### ChannelType2APIType

定义在 `common/api_type.go`，是 `ChannelType -> APIType` 的桥。漏掉这里通常会导致请求走错 adaptor 或取不到 adaptor。

### Adaptor

定义在 `relay/channel/adapter.go`。普通 relay channel 必须实现：

- 初始化运行时信息。
- 构造上游 URL。
- 设置上游请求头。
- 将 OpenAI / Claude / Gemini / Embedding / Image / Audio / Rerank / Responses 请求转换成 provider 需要的格式。
- 发起请求。
- 解析响应并返回 usage。
- 返回静态模型列表和渠道名。

### RelayInfo

定义在 `relay/common/relay_info.go`。它是一条 relay 请求的核心上下文，包含：

- 请求格式和 relay mode。
- 原始模型名和上游模型名。
- 是否流式。
- 用户、token、分组、quota 信息。
- channel type、channel id、base URL、API key、API type、settings、override。
- 计费、usage、thinking、cache、responses tools 等辅助信息。

遇到 relay 行为不确定时，先问：“这个字段在 `RelayInfo` 里叫什么？在哪里被写入？在哪里被读取？”

## 常见需求的定位方法

### 新增或调整一个 HTTP 接口

先看：

```text
router/api-router.go
controller/<domain>.go
service/<domain>.go
model/<domain>.go
```

如果需要权限或 middleware，再看 `middleware/`。

### 改一个 AI 请求的转发行为

先判断入口格式：

- OpenAI chat/completions：`relay/compatible_handler.go`
- OpenAI Responses：`relay/responses_handler.go`
- Claude：`relay/claude_handler.go`
- Gemini：`relay/gemini_handler.go`
- Embedding：`relay/embedding_handler.go`
- Image：`relay/image_handler.go`
- Audio：`relay/audio_handler.go`
- Rerank：`relay/rerank_handler.go`

然后追：

```text
relay.GetAdaptor -> relay/channel/<provider>/adaptor.go
```

### 改某个 provider 的鉴权、URL、请求字段、响应解析

先看：

```text
relay/channel/<provider>/adaptor.go
relay/channel/<provider>/dto.go
relay/channel/<provider>/relay-*.go
```

如果它复用 OpenAI/Claude/Gemini handler，再看对应基座目录。

### 改渠道选择、负载均衡、自动禁用

先看：

```text
middleware/distributor.go
model/channel.go
model/ability.go
model/channel_cache.go
```

再根据是否涉及用户分组、模型能力、token 限制去看 `service/` 和 `setting/`。

### 改计费、倍率、quota、usage

先看：

```text
service/text_quota.go
service/tool_billing.go
service/tiered_settle*.go
model/pricing.go
setting/ratio_setting/
```

如果涉及表达式计费，必须先读：

```text
pkg/billingexpr/expr.md
```

这是项目规则，不要跳过。

### 改模型列表、模型 owner、模型能力

先看：

```text
controller/model.go
model/model_meta.go
model/ability.go
model/pricing.go
relay/channel/<provider>/constants.go
```

模型是否出现在 `/v1/models`、后台 dashboard、价格页，可能分别来自不同数据源。

### 改后台渠道页面

默认前端看：

```text
web/default/src/features/channels/
```

经典前端看：

```text
web/classic/src/constants/channel.constants.js
web/classic/src/components/table/channels/
```

如果新增了后端枚举，通常需要同步前端枚举、图标、提示、表单字段和 i18n。

### 改系统设置

先看：

```text
setting/
controller/option*.go 或相关 settings controller
web/default/src/features/system-settings/
```

然后追运行时读取这些 setting 的 service/relay 代码。

### 改数据库结构

先看：

```text
model/<domain>.go
model/main.go
```

注意跨数据库兼容：SQLite、MySQL、PostgreSQL 都要能跑。优先 GORM，raw SQL 要处理方言差异。

## 关键横切约束

### JSON wrapper

业务代码不要直接调用 `encoding/json` 的 marshal/unmarshal。使用：

- `common.Marshal`
- `common.Unmarshal`
- `common.UnmarshalJsonStr`
- `common.DecodeJson`
- `common.GetJsonType`

类型上可以引用 `json.RawMessage`、`json.Number` 等，但实际序列化/反序列化要走 wrapper。

### 数据库兼容

所有数据库代码要同时兼容：

- SQLite
- MySQL >= 5.7.8
- PostgreSQL >= 9.6

优先使用 GORM。raw SQL 不可避免时要注意：

- PostgreSQL 使用 `"column"`。
- MySQL/SQLite 使用 `` `column` ``。
- reserved word 列如 `group`、`key` 使用项目已有公共变量。
- 布尔值在 PostgreSQL 和 MySQL/SQLite 表达不同。
- SQLite 不支持很多 `ALTER COLUMN` 行为。

### 上游请求 DTO 零值保留

relay 请求结构里，可选标量字段要使用指针类型加 `omitempty`。

语义必须是：

```text
客户端没传 -> nil -> marshal 时省略
客户端显式传 0 / false -> 非 nil 指针 -> marshal 时保留并发给上游
```

不要用非指针标量加 `omitempty` 表示可选参数。

### StreamOptions

新增或调整 channel / stream 行为时，要确认 provider 是否支持 `StreamOptions`。支持才加入 `relay/common/relay_info.go` 里的 `streamSupportedChannels`。

### 受保护项目信息

不要删除、替换、改名或移除项目 policy 保护的项目名、组织名、版权、元数据、包路径、镜像名、文档归属等信息。遇到这类需求要拒绝。

### 前端包管理器

`web/default/` 优先使用 Bun：

```bash
bun install
bun run dev
bun run build
bun run i18n:sync
```

## 读代码的推荐顺序

如果你完全不知道一个需求该从哪里下手，按这个顺序扫：

1. 找入口：`router/` 里有没有对应路径。
2. 找 controller：路径对应哪个 controller 函数。
3. 找上下文：controller 里生成或读取了哪些 context / RelayInfo / request DTO。
4. 找核心执行者：普通业务去 `service/model`，AI 请求去 `relay/channel`。
5. 找横切影响：是否涉及 setting、计费、日志、缓存、i18n、前端配置。
6. 找测试：同目录或相邻目录的 `_test.go`。

这个顺序能避免一开始就陷进某个 provider 的细节里。

## 常用搜索关键词

```bash
rg -n "func Relay|RelayInfo|InitChannelMeta|GetAdaptor|ChannelType2APIType"
rg -n "ChannelTypeYourName|APITypeYourName|ChannelBaseURLs|ChannelTypeNames"
rg -n "Path2RelayMode|RelayModeChatCompletions|RelayModeResponses"
rg -n "DoResponse|ConvertOpenAIRequest|SetupRequestHeader|GetRequestURL" relay/channel
rg -n "GetModelList|ChannelName|ModelList" relay/channel
rg -n "ApplyParamOverride|HeaderOverride|ParamOverride" relay
rg -n "quota|billing|Usage|PreConsumed|ModelRatio" service model setting
```

## 开发前快速判断清单

开始做任何需求前，先问这些问题：

1. 这是后台管理接口、用户接口、还是 AI relay 请求？
2. 如果是 AI relay，它的入口格式是 OpenAI、Claude、Gemini、Responses、Image、Audio、Rerank，还是异步任务？
3. 它依赖哪个 channel？该 channel 是独立 adaptor，还是复用 OpenAI/Claude/Gemini？
4. 是否涉及数据库结构或查询？如果是，三种数据库是否兼容？
5. 是否涉及 JSON marshal/unmarshal？如果是，是否使用 `common/json.go` wrapper？
6. 是否涉及可选请求参数？显式零值是否会被保留？
7. 是否涉及计费或倍率？是否需要读 `pkg/billingexpr/expr.md`？
8. 是否需要同步默认前端和 classic 前端？
9. 是否需要补 i18n？
10. 是否有相邻测试可复用或需要新增？

## 高风险区域

### `constant/channel.go`

渠道编号和默认 base URL 与数据库、前端、模型能力相关。不要重排已有编号。

### `relay/channel/openai/adaptor.go`

这是很多兼容渠道的共享基座，逻辑重且影响面大。改这里要非常谨慎，优先确认是否可以在具体 provider adaptor 中做局部处理。

### `middleware/distributor.go`

它决定请求选哪个渠道。改动会影响所有 relay 请求的路由、负载均衡、重试和可用性。

### `service/text_quota.go` 和 tiered billing

计费影响真实 quota。改前要明确 usage 语义、缓存 token、image/audio/tool surcharge、group ratio、tiered expr 是否一致。

### `model/main.go`

迁移和数据库兼容风险集中地。不要只在 SQLite 或 MySQL 上想当然。

## 验证建议

不同需求需要不同验证层级：

- 普通 Go 改动：优先跑相关 package 的 `go test`。
- relay/channel 改动：至少跑对应 `relay/channel/<provider>`、`relay` 相关测试。
- 计费改动：跑 `service/*quota*`、`service/*billing*`、`service/*tiered*` 相关测试。
- 前端改动：在 `web/default/` 跑 `bun run build`，i18n 相关跑 `bun run i18n:sync` 或项目已有 i18n 脚本。
- 跨数据库改动：不要只依赖默认 SQLite，需要检查 SQL/GORM 是否对 MySQL/PostgreSQL 也成立。

## 当前全局理解的边界

这份文档来自一次只读全局扫描和对关键文件的人工复核。它覆盖了项目主架构、relay 主链路、渠道/adaptor 机制、后台和前端同步点、关键约束。

它没有完整展开每一个 provider 的细节，也没有深入每个 setting 页面、每张表、每个计费表达式。后续遇到具体需求时，应该以本文作为入口地图，再对相关局部做精读。
