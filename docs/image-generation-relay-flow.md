# OpenAI 兼容生图请求链路

本文档整理当前后端里一个生图请求的主要流转路径，重点覆盖 OpenAI 兼容接口：

```http
POST /v1/images/generations
POST /v1/images/edits
```

如果请求来自画布入口：

```http
POST /api/canvas/relay/images/generations
POST /api/canvas/relay/images/edits
```

最终也会被改写到 `/v1/images/...` 并进入同一条 OpenAI Image relay 链路。

## 结论总览

普通生图请求的主链路：

```text
router/relay-router.go
-> relay 全局中间件
-> /v1 分组中间件
-> middleware.Distribute 选渠道
-> controller.Relay
-> helper.GetAndValidateRequest
-> relaycommon.GenRelayInfo
-> 敏感词检查 / token 预估 / 价格计算 / 预扣费
-> controller.Relay retry loop
-> relay.ImageHelper
-> relay.GetAdaptor
-> adaptor.ConvertImageRequest
-> adaptor.DoRequest
-> channel.DoApiRequest 或 channel.DoFormRequest
-> 上游 provider
-> adaptor.DoResponse
-> OpenaiImageHandler 或 OpenaiImageStreamHandler
-> service.PostTextConsumeQuota
```

主入口不在 `router/api-router.go` 的普通 `/api` 管理接口里，而是在 `router/relay-router.go` 的 `/v1` relay 路由里。`router/api-router.go` 里只额外注册了画布专用入口 `/api/canvas/relay/...`。

## 一、路由入口

### 1. Relay 全局中间件

文件：`router/relay-router.go`

`SetRelayRouter(router *gin.Engine)` 会先给整个 relay router 挂全局中间件：

```go
router.Use(middleware.CORS())
router.Use(middleware.DecompressRequestMiddleware())
router.Use(middleware.BodyStorageCleanup())
router.Use(middleware.StatsMiddleware())
```

这些中间件负责：

- 跨域处理。
- 解压请求体。
- 请求体缓存与清理。
- 请求统计。

### 2. `/v1` 分组中间件

同一文件里，`/v1` relay 分组继续挂：

```go
relayV1Router.Use(middleware.RouteTag("relay"))
relayV1Router.Use(middleware.SystemPerformanceCheck())
relayV1Router.Use(middleware.TokenAuth())
relayV1Router.Use(middleware.ModelRequestRateLimit())
```

这些中间件负责：

- 标记当前请求为 relay 请求。
- 系统性能状态检查。
- token 鉴权。
- 按模型限流。

### 3. HTTP relay 子路由和渠道分发

`/v1` 下的 HTTP relay 子路由会先跑渠道分发：

```go
httpRouter := relayV1Router.Group("")
httpRouter.Use(middleware.Distribute())
```

然后图片相关路由注册为：

```go
httpRouter.POST("/images/generations", func(c *gin.Context) {
    controller.Relay(c, types.RelayFormatOpenAIImage)
})

httpRouter.POST("/images/edits", func(c *gin.Context) {
    controller.Relay(c, types.RelayFormatOpenAIImage)
})
```

补充：

- `POST /v1/edits` 也被注册到 `RelayFormatOpenAIImage`，这是旧式兼容入口。
- `POST /v1/images/variations` 当前是 `controller.RelayNotImplemented`，没有实现。

## 二、`middleware.Distribute()` 做什么

文件：`middleware/distributor.go`

`Distribute()` 是生图请求真正进入 controller 前的关键步骤。它负责：

1. 从请求里读取 `model`。
2. 校验 token 是否允许访问该模型。
3. 根据用户分组、模型、渠道可用性选择一个渠道。
4. 把选中的渠道信息写入 gin context。
5. 读取渠道 key、base URL、模型映射、参数覆盖、header 覆盖等配置。

核心写入的信息包括：

```text
channel_id
channel_type
channel_name
channel_key
channel_base_url
channel_model_mapping
channel_param_override
channel_header_override
status_code_mapping
original_model
```

对 `/v1/images/generations` 有一个分发阶段的特殊默认：

```go
if strings.HasPrefix(c.Request.URL.Path, "/v1/images/generations") {
    modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "dall-e")
}
```

注意：这个默认值主要用于渠道选择。后续 `GetAndValidOpenAIImageRequest()` 仍要求业务请求体里有 `model`，否则会返回 `model is required`。

## 三、进入 `controller.Relay`

文件：`controller/relay.go`

路由命中后进入：

```go
controller.Relay(c, types.RelayFormatOpenAIImage)
```

`Relay()` 是所有同步 relay 请求的统一控制器。图片请求在这里会经历这些步骤。

### 1. 统一错误包装

`Relay()` 内部先注册 defer。如果后续出现 `newAPIError`，会按 OpenAI 错误格式返回：

```json
{
  "error": {
    "message": "...",
    "type": "...",
    "code": "..."
  }
}
```

### 2. 请求解析与校验

调用：

```go
request, err := helper.GetAndValidateRequest(c, relayFormat)
```

对于 `types.RelayFormatOpenAIImage`，会进入：

```go
GetAndValidOpenAIImageRequest(c, relayMode)
```

`relayMode` 来自：

```go
relayconstant.Path2RelayMode(c.Request.URL.Path)
```

路径和 relay mode 的对应关系：

```text
/v1/images/generations -> RelayModeImagesGenerations
/v1/images/edits       -> RelayModeImagesEdits
```

### 3. 生成 `RelayInfo`

调用：

```go
relayInfo, err := relaycommon.GenRelayInfo(c, relayFormat, request, ws)
```

对于图片请求会进入：

```go
GenRelayInfoImage(c, request)
```

`RelayInfo` 是后续所有 relay 层共享的上下文对象，包含：

- token 信息。
- 用户信息。
- 分组信息。
- 原始模型名。
- 请求路径。
- relay format。
- relay mode。
- 是否流式。
- 请求 DTO。
- 计费上下文。
- 重试状态。

其中图片请求是否流式由 `dto.ImageRequest.IsStream()` 决定：

```go
func (i *ImageRequest) IsStream(c *gin.Context) bool {
    return i.Stream != nil && *i.Stream
}
```

## 四、图片请求 DTO 与校验规则

文件：`dto/openai_image.go`

图片请求结构体：

```go
type ImageRequest struct {
    Model          string          `json:"model"`
    Prompt         string          `json:"prompt" binding:"required"`
    N              *uint           `json:"n,omitempty"`
    Size           string          `json:"size,omitempty"`
    Quality        string          `json:"quality,omitempty"`
    ResponseFormat string          `json:"response_format,omitempty"`
    Stream         *bool           `json:"stream,omitempty"`
    Image          json.RawMessage `json:"image,omitempty"`
    Mask           json.RawMessage `json:"mask,omitempty"`
    Watermark      *bool           `json:"watermark,omitempty"`
    Extra          map[string]json.RawMessage `json:"-"`
}
```

文件：`relay/helper/valid_request.go`

### `/v1/images/generations`

JSON 请求会被解析到 `dto.ImageRequest`。主要校验和默认值：

- `model` 必填。
- `size` 不能包含乘号 `×`，必须用 `x`。
- `dall-e` / `dall-e-2` 支持 `256x256`、`512x512`、`1024x1024`，默认 `1024x1024`。
- `dall-e-3` 支持 `1024x1024`、`1024x1792`、`1792x1024`，默认 `1024x1024`。
- `dall-e-3` 默认 `quality=standard`。
- `gpt-image-1` 默认 `quality=auto`。
- `n` 为空或为 0 时默认 `1`。

### `/v1/images/edits`

如果是 `multipart/form-data`：

- 会通过 `common.ParseMultipartFormReusable(c)` 解析表单。
- 从表单读取 `prompt`、`model`、`n`、`quality`、`size`、`stream`、`watermark`。
- `n` 为空或为 0 时默认 `1`。
- 如果 `model == "gpt-image-1"` 且没有传 `quality`，默认 `quality=standard`。

如果不是 multipart，则回退到普通 JSON 解析逻辑。

## 五、敏感词、token 预估、价格与预扣费

文件：`controller/relay.go`

`Relay()` 在真正请求上游前会先处理计费前置逻辑。

### 1. 获取 token meta

图片请求会调用：

```go
request.GetTokenCountMeta()
```

对应实现：

```go
func (i *ImageRequest) GetTokenCountMeta() *types.TokenCountMeta {
    return &types.TokenCountMeta{
        CombineText:     i.Prompt,
        MaxTokens:       1584,
        ImagePriceRatio: sizeRatio * qualityRatio,
    }
}
```

这里会根据 `model`、`size`、`quality` 计算 `ImagePriceRatio`。例如 `dall-e-3` 的 `hd` 和大尺寸会有更高倍率。

注意：`n` 不在 `GetTokenCountMeta()` 里计算，避免被重复计费。图片数量会在 `relay.ImageHelper()` 的后置阶段作为 `OtherRatio("n")` 处理。

### 2. 敏感词检查

如果系统配置开启 prompt 敏感词检查，会检查：

```text
meta.CombineText
```

也就是图片请求里的 `prompt`。

### 3. token 预估与价格计算

依次调用：

```go
service.EstimateRequestToken(c, meta, relayInfo)
helper.ModelPriceHelper(c, relayInfo, tokens, meta)
```

输出的 `priceData` 会放进 `relayInfo.PriceData`。

### 4. 预扣费

非免费模型会调用：

```go
service.PreConsumeBilling(c, priceData.QuotaToPreConsume, relayInfo)
```

如果后续请求失败，`Relay()` 的 defer 会触发退款：

```go
relayInfo.Billing.Refund(c)
```

## 六、重试循环

文件：`controller/relay.go`

预扣费成功后进入 retry loop：

```go
for ; retryParam.GetRetry() <= common.RetryTimes; retryParam.IncreaseRetry() {
    channel, channelErr := getChannel(c, relayInfo, retryParam)
    ...
    c.Request.Body = io.NopCloser(bodyStorage)
    ...
    newAPIError = relayHandler(c, relayInfo)
}
```

每次循环会：

1. 获取当前渠道。
2. 记录使用过的渠道。
3. 重新设置请求 body，保证重试时还能读取原始请求。
4. 根据 relay format 和 relay mode 分派到具体 handler。
5. 出错后判断是否允许重试。
6. 必要时自动禁用异常渠道。

图片请求会被分派到：

```go
relay.ImageHelper(c, info)
```

## 七、进入 `relay.ImageHelper`

文件：`relay/image_handler.go`

这是图片请求的核心 relay 层。

### 1. 初始化渠道元信息

```go
info.InitChannelMeta(c)
```

它会从 gin context 里读取 `Distribute()` 写入的渠道信息，并填充到 `info.ChannelMeta`：

```text
ChannelType
ChannelId
ChannelBaseUrl
ApiType
ApiVersion
ApiKey
Organization
ChannelSetting
ChannelOtherSettings
ParamOverride
HeadersOverride
UpstreamModelName
SupportStreamOptions
```

### 2. 深拷贝请求

```go
request, err := common.DeepCopy(imageReq)
```

这样后续模型映射、参数转换不会直接污染原始请求对象。

### 3. 模型映射

```go
helper.ModelMappedHelper(c, info, request)
```

如果渠道配置了模型映射，这里会把用户请求模型改成上游真实模型名。

示例：

```text
客户端请求 model: image-fast
渠道模型映射: {"image-fast":"gpt-image-1"}
上游请求 model: gpt-image-1
```

### 4. 选择 provider adaptor

```go
adaptor := GetAdaptor(info.ApiType)
adaptor.Init(info)
```

`GetAdaptor()` 位于 `relay/relay_adaptor.go`，会按 `ApiType` 返回不同实现：

```text
APITypeOpenAI     -> relay/channel/openai.Adaptor
APITypeAli        -> relay/channel/ali.Adaptor
APITypeGemini     -> relay/channel/gemini.Adaptor
APITypeVertexAi   -> relay/channel/vertex.Adaptor
APITypeReplicate  -> relay/channel/replicate.Adaptor
APITypeMiniMax    -> relay/channel/minimax.Adaptor
...
```

从这里开始，不同 provider 的请求格式、URL、响应处理会出现分叉。

## 八、请求体转换

文件：`relay/image_handler.go`

如果开启了 pass-through：

```go
model_setting.GetGlobalSettings().PassThroughRequestEnabled
info.ChannelSetting.PassThroughBodyEnabled
```

则直接把原始 body 传给上游。

否则会调用：

```go
convertedRequest, err := adaptor.ConvertImageRequest(c, info, *request)
```

### OpenAI 兼容 adaptor

文件：`relay/channel/openai/adaptor.go`

对 `/v1/images/generations`：

```go
return request, nil
```

也就是基本透传 `dto.ImageRequest`，再由 `ImageHelper()` marshal 成 JSON。

对 multipart `/v1/images/edits`：

- 复用已解析的 multipart form。
- 写入所有非文件字段。
- 查找并复制 `image`、`image[]`、`image[n]`。
- 如果存在 `mask`，复制 `mask`。
- 重建 multipart body。
- 重设 `Content-Type` 为新的 multipart boundary。

### 其他 provider adaptor

其他渠道会在自己的 `ConvertImageRequest()` 中把 OpenAI 风格请求转换成上游格式，例如：

- `relay/channel/ali`
- `relay/channel/gemini`
- `relay/channel/vertex`
- `relay/channel/replicate`
- `relay/channel/minimax`
- `relay/channel/zhipu`

这些实现通常会处理：

- 上游 URL 或 endpoint 差异。
- prompt 字段映射。
- size、quality、n 的字段映射。
- 图片编辑时的图片上传、base64 转换或 URL 上传。
- 上游响应格式转回 OpenAI Image 格式。

## 九、应用参数覆盖并生成上游 body

文件：`relay/image_handler.go`

如果 `convertedRequest` 不是 `*bytes.Buffer`，会被 marshal 成 JSON：

```go
jsonData, err := common.Marshal(convertedRequest)
```

如果渠道配置了参数覆盖：

```go
jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
```

然后包装成 outbound body：

```go
body, size, closer, err := relaycommon.NewOutboundJSONBody(jsonData)
info.UpstreamRequestBodySize = size
requestBody = body
```

## 十、请求上游

文件：`relay/image_handler.go`

调用：

```go
resp, err := adaptor.DoRequest(c, info, requestBody)
```

### OpenAI 兼容 adaptor 的分支

文件：`relay/channel/openai/adaptor.go`

```go
if info.RelayMode == RelayModeImagesEdits && !isJSONRequest(c) {
    return channel.DoFormRequest(a, c, info, requestBody)
}
return channel.DoApiRequest(a, c, info, requestBody)
```

也就是：

```text
/v1/images/generations JSON -> channel.DoApiRequest
/v1/images/edits JSON       -> channel.DoApiRequest
/v1/images/edits multipart  -> channel.DoFormRequest
```

### `channel.DoApiRequest`

文件：`relay/channel/api_request.go`

主要步骤：

1. `a.GetRequestURL(info)` 生成上游 URL。
2. `http.NewRequest(c.Request.Method, fullRequestURL, requestBody)` 创建请求。
3. `applyUpstreamContentLength(req, info)` 设置 Content-Length。
4. `a.SetupRequestHeader(c, &headers, info)` 设置鉴权和基础 header。
5. `processHeaderOverride(info, c)` 处理渠道 header override。
6. `applyHeaderOverrideToRequest(req, headerOverride)` 应用覆盖。
7. `doRequest(c, req, info)` 真正请求上游。

### `doRequest`

同一文件中：

- 如果渠道配置了代理，使用 `service.NewProxyHttpClient()`。
- 否则使用全局 HTTP client。
- 如果是流式请求，会先设置 SSE 响应头，并按配置开启 ping 保活。
- 最后执行 `client.Do(req)`。
- 如果上游返回 request id，会写入当前 context。

## 十一、处理上游 HTTP 状态

文件：`relay/image_handler.go`

上游响应回来后：

```go
httpResp = resp.(*http.Response)
info.IsStream = info.IsStream || strings.HasPrefix(httpResp.Header.Get("Content-Type"), "text/event-stream")
```

如果状态码不是 `200 OK`：

- Replicate 的 `201 Created` 会被当成成功。
- 其他非 200 会进入 `service.RelayErrorHandler()`，转换成统一错误。
- 如果配置了状态码映射，会调用 `service.ResetStatusCode()`。

## 十二、处理响应并返回客户端

文件：`relay/image_handler.go`

调用：

```go
usage, newAPIError := adaptor.DoResponse(c, httpResp, info)
```

### OpenAI 兼容 adaptor

文件：`relay/channel/openai/adaptor.go`

图片请求会进入：

```go
case RelayModeImagesGenerations, RelayModeImagesEdits:
    if info.IsStream {
        usage, err = OpenaiImageStreamHandler(c, info, resp)
    } else {
        usage, err = OpenaiImageHandler(c, info, resp)
    }
```

### 非流式响应

文件：`relay/channel/openai/relay_image.go`

`OpenaiImageHandler()` 会：

1. 读取上游 response body。
2. 解析 `dto.SimpleResponse`。
3. 检查上游是否返回 OpenAI error。
4. 将原始上游响应 body 写回客户端。
5. 归一化 OpenAI Image API 的 usage 字段。
6. 返回 `*dto.Usage` 给后续计费。

### 流式响应

同一文件中，`OpenaiImageStreamHandler()` 会：

1. 如果上游返回错误 HTTP 状态，回退到 `OpenaiImageHandler()`。
2. 如果上游不是 `text/event-stream`，用 `OpenaiImageJSONAsStreamHandler()` 把 JSON 图片结果包装成 SSE。
3. 如果上游是 SSE，使用 `helper.StreamScannerHandler()` 逐块读取。
4. 从每个 chunk 里解析 usage。
5. 重新写出 `event:` 和 `data:`。
6. 结束时向客户端发送 `data: [DONE]`。

## 十三、后置计费与日志

文件：`relay/image_handler.go`

响应处理成功后，`ImageHelper()` 会做后置结算准备：

```go
imageN := uint(1)
if request.N != nil {
    imageN = *request.N
}

if info.PriceData.UsePrice {
    if _, hasN := info.PriceData.OtherRatios["n"]; !hasN {
        info.PriceData.AddOtherRatio("n", float64(imageN))
    }
}
```

然后保证 usage 至少有基础值：

```go
if usage.(*dto.Usage).TotalTokens == 0 {
    usage.(*dto.Usage).TotalTokens = 1
}
if usage.(*dto.Usage).PromptTokens == 0 {
    usage.(*dto.Usage).PromptTokens = 1
}
```

最后记录日志内容：

```text
大小 <size>
品质 <quality>
生成数量 <n>
```

并调用：

```go
service.PostTextConsumeQuota(c, info, usage.(*dto.Usage), logContent)
```

完成实际结算、日志落库和额度更新。

## 十四、失败、退款与重试

文件：`controller/relay.go`

如果任意步骤失败：

1. `processChannelError()` 记录渠道错误。
2. 根据错误类型和状态码判断是否自动禁用渠道。
3. `shouldRetry()` 判断是否进入下一次重试。
4. 如果最终失败，预扣费会退款。
5. 可能按策略收取违规费用。
6. 返回 OpenAI 格式错误给客户端。

核心失败处理：

```go
if newAPIError != nil {
    newAPIError = service.NormalizeViolationFeeError(newAPIError)
    if relayInfo.Billing != nil {
        relayInfo.Billing.Refund(c)
    }
    service.ChargeViolationFeeIfNeeded(c, relayInfo, newAPIError)
}
```

## 十五、画布入口

文件：`router/api-router.go`

画布 relay 路由注册在 `/api` 路由里：

```go
apiRouter.POST("/canvas/relay-token", middleware.UserAuth(), controller.EnsureCanvasRelayToken)

canvasRelayRoute := apiRouter.Group("/canvas/relay")
canvasRelayRoute.Use(middleware.UserAuth(), controller.CanvasRelayAuth(), middleware.ModelRequestRateLimit())
{
    canvasRelayRoute.POST("/images/generations", middleware.Distribute(), controller.CanvasRelayOpenAI)
    canvasRelayRoute.POST("/images/edits", middleware.Distribute(), controller.CanvasRelayOpenAI)
}
```

文件：`controller/canvas_relay.go`

`CanvasRelayOpenAI()` 会把路径改写为 `/v1` 开头：

```go
c.Request.URL.Path = "/v1" + relayPath
```

然后按 relay mode 分派。图片请求会进入：

```go
Relay(c, types.RelayFormatOpenAIImage)
```

因此画布生图链路是：

```text
/api/canvas/relay/images/generations
-> UserAuth
-> CanvasRelayAuth
-> ModelRequestRateLimit
-> Distribute
-> CanvasRelayOpenAI
-> 改写为 /v1/images/generations
-> controller.Relay(OpenAIImage)
-> 后续同主链路
```

`CanvasRelayAuth()` 会：

- 读取当前登录用户。
- 获取或创建画布 relay token。
- 调用 `middleware.SetupContextForToken()` 写入 token 上下文。
- 把请求 Authorization header 设置成内部 relay token。
- 设置当前用户分组。

## 十六、Midjourney 生图是另一条链路

如果请求是：

```http
POST /mj/submit/imagine
```

它不走 `relay.ImageHelper()`，而是走 Midjourney task/proxy 链路。

路由位于 `router/relay-router.go`：

```go
relayMjRouter.POST("/submit/imagine", controller.RelayMidjourney)
```

大致链路：

```text
/mj/submit/imagine
-> RouteTag("relay")
-> SystemPerformanceCheck
-> TokenAuth
-> Distribute
-> controller.RelayMidjourney
-> relay/mjproxy_handler.go 相关逻辑
-> Midjourney 上游或代理
-> 任务记录 / 查询 / 图片代理
```

所以排查时要先看请求路径：

```text
/v1/images/generations -> OpenAI Image relay 链路
/api/canvas/relay/images/generations -> Canvas 改写后进入 OpenAI Image relay 链路
/mj/submit/imagine -> Midjourney 链路
```

## 十七、Responses 内建生图不是这条链路

如果客户端调用的是：

```http
POST /v1/responses
```

并在 Responses API 里使用内建 `image_generation` tool，那么它会走：

```text
controller.Relay(OpenAIResponses)
-> relay.ResponsesHelper
-> relay/channel/openai/relay_responses.go
```

这不是 `/v1/images/generations` 的 `relay.ImageHelper()` 链路。

相关计费标记会通过 context 设置：

```go
c.Set("image_generation_call", true)
c.Set("image_generation_call_quality", ...)
c.Set("image_generation_call_size", ...)
```

然后由 tool billing 相关逻辑处理。

## 十八、排查入口速查

### 路由层

```text
router/relay-router.go       # /v1/images/generations 和 /v1/images/edits
router/api-router.go         # /api/canvas/relay/images/generations
controller/canvas_relay.go   # Canvas 路径改写
```

### 分发与鉴权

```text
middleware/distributor.go    # 读取 model、选择渠道、设置 channel context
middleware/auth.go           # token 鉴权相关，按实际函数名继续追
```

### 请求解析

```text
relay/helper/valid_request.go # GetAndValidateRequest / GetAndValidOpenAIImageRequest
dto/openai_image.go           # ImageRequest / ImageResponse
relay/constant/relay_mode.go  # Path2RelayMode
```

### 统一 controller

```text
controller/relay.go           # Relay / retry / 预扣费 / 错误处理
```

### 图片 relay 核心

```text
relay/image_handler.go        # ImageHelper
relay/relay_adaptor.go        # GetAdaptor
relay/channel/adapter.go      # Adaptor interface
relay/channel/api_request.go  # DoApiRequest / DoFormRequest / doRequest
```

### OpenAI 兼容 provider

```text
relay/channel/openai/adaptor.go     # ConvertImageRequest / DoRequest / DoResponse
relay/channel/openai/relay_image.go # OpenaiImageHandler / OpenaiImageStreamHandler
```

### 其他 provider

```text
relay/channel/ali/
relay/channel/gemini/
relay/channel/vertex/
relay/channel/replicate/
relay/channel/minimax/
relay/channel/zhipu/
relay/channel/volcengine/
```

## 十九、最短调用视角

如果只想从调用栈角度看核心函数，可以按这个顺序追：

```text
SetRelayRouter
Distribute
controller.Relay
helper.GetAndValidateRequest
helper.GetAndValidOpenAIImageRequest
relaycommon.GenRelayInfo
service.EstimateRequestToken
helper.ModelPriceHelper
service.PreConsumeBilling
relay.ImageHelper
RelayInfo.InitChannelMeta
helper.ModelMappedHelper
relay.GetAdaptor
adaptor.ConvertImageRequest
adaptor.DoRequest
channel.DoApiRequest / channel.DoFormRequest
adaptor.DoResponse
OpenaiImageHandler / OpenaiImageStreamHandler
service.PostTextConsumeQuota
```

