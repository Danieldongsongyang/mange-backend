# AgnesAI 视频生成失败原因分析

## 背景

用户在 Canvas 前端使用远程后端的视频生成功能时，请求：

```text
POST /api/canvas/relay/videos
```

后端日志显示任务创建失败：

```text
[SYS] model price not found: agnes-video-v2.0
[INFO] 用户 1 需要预扣费 ＄75.000000 (funding=wallet)
[ERR] channel error (channel #3, status code: 500): {"code":"fail_to_fetch_task","message":"{\"error\":{\"message\":\"litellm.InternalServerError: InternalServerError: OpenAIException - Object of type UploadFile is not JSON serializable\",\"type\":null,\"param\":null,\"code\":\"500\"}}","data":null}
[INFO] 用户 1 请求失败, 返还预扣费（token_quota=＄75.000000, funding=wallet）
```

## 结论

这次失败不是用户余额不足，也不是后端预扣费本身失败。

真正导致任务创建失败的是：Canvas 前端发送的视频请求是 `multipart/form-data`，其中参考图通过 `input_reference[]` 文件字段上传；当前后端把 AgnesAI 视频通道归到 Sora/OpenAI 视频任务适配器处理，该适配器会把 multipart 文件原样转发给上游。AgnesAI 上游/LiteLLM 在处理该请求时试图把 `UploadFile` 对象序列化成 JSON，于是报错：

```text
Object of type UploadFile is not JSON serializable
```

也就是说，AgnesAI 视频接口当前收到的是它不能处理的文件对象，而不是它期望的 URL/base64/data URL 字符串。

## 现象拆解

### 1. `model price not found` 是计费配置问题

日志：

```text
model price not found: agnes-video-v2.0
```

说明当前模型价格配置里没有 `agnes-video-v2.0` 的明确价格。

相关代码：

- `relay/relay_task.go`
- `relay/helper/price.go`
- `setting/ratio_setting/model_ratio.go`

当前默认价格表里有类似：

```go
"sora-2": 0.3,
"sora-2-pro": 0.5,
"veo-3.0-generate-001": 0.4,
```

但没有：

```go
"agnes-video-v2.0"
```

因此系统走了 fallback/默认预扣逻辑，最终预扣：

```text
＄75.000000
```

这个问题会导致预扣金额异常偏高，但它不是本次 500 的直接原因，因为日志后面显示预扣成功，并且失败后已经退款。

### 2. 视频任务强制预扣，不走信任旁路

视频生成走异步任务 relay，代码中会设置：

```go
info.ForcePreConsume = true
```

相关代码：

- `relay/relay_task.go`
- `service/billing_session.go`

因此视频任务必须先预扣预计费用。它不会像普通图片请求那样，因为用户额度充足就走“信任且不需要预扣费”的旁路。

本次日志里的：

```text
用户 1 需要预扣费 ＄75.000000
```

属于异步视频任务的正常计费流程，只是金额因为模型价格未配置而偏高。

### 3. 真正失败点是上游 500

日志中的关键错误：

```text
litellm.InternalServerError: InternalServerError: OpenAIException - Object of type UploadFile is not JSON serializable
```

这表示请求已经发到上游，上游在处理请求体时失败。

根据代码链路：

1. Canvas 前端请求 `/api/canvas/relay/videos`。
2. 后端 `controller/canvas_relay.go` 把路径重写为 `/v1/videos`。
3. 请求进入 `RelayTask` / `RelayTaskSubmit`。
4. `GetTaskPlatform` 根据当前通道类型得到 AgnesAI 的 channel type。
5. `GetTaskAdaptor` 把 `ChannelTypeAgnesAI` 归到 Sora/OpenAI 视频任务适配器。
6. Sora/OpenAI 视频任务适配器原样处理 multipart 文件并转发。

相关代码：

- `controller/canvas_relay.go`
- `relay/relay_task.go`
- `relay/relay_adaptor.go`
- `relay/channel/task/sora/adaptor.go`

关键映射位于 `relay/relay_adaptor.go`：

```go
case constant.ChannelTypeSora, constant.ChannelTypeOpenAI, constant.ChannelTypeAgnesAI:
    return &tasksora.TaskAdaptor{}
```

也就是说，AgnesAI 视频现在复用了 Sora 的 task adaptor。

## 前端请求形态

Canvas 前端普通视频任务创建逻辑位于：

```text
/Users/a1/Desktop/my-canvas/web/src/services/api/video.ts
```

请求构造大致如下：

```ts
const body = new FormData();
body.append("model", model);
body.append("prompt", prompt);
body.append("seconds", normalizeVideoSeconds(config.videoSeconds));
body.append("size", size);
body.append("resolution_name", normalizeVideoResolution(config.vquality));
body.append("preset", "normal");
files.forEach((file) => body.append("input_reference[]", file));
```

也就是说，只要用户连了参考图，前端就会把图片作为 multipart 文件字段 `input_reference[]` 发送。

## 当前后端处理方式

当前 Sora task adaptor 的 `BuildRequestBody` 对 multipart 请求会重新构造 multipart：

```go
writer := multipart.NewWriter(&buf)
writer.WriteField("model", info.UpstreamModelName)
...
for fieldName, fileHeaders := range formData.File {
    for _, fh := range fileHeaders {
        ...
        part, err := writer.CreatePart(h)
        ...
        io.Copy(part, f)
    }
}
```

相关文件：

```text
relay/channel/task/sora/adaptor.go
```

这个行为适合真正支持 OpenAI/Sora multipart 上传的上游，但不适合 AgnesAI 当前的视频接口。

## AgnesAI 当前不兼容点

AgnesAI 视频接口在当前上游表现上，不接受被原样透传的 multipart 文件对象。

上游报错：

```text
Object of type UploadFile is not JSON serializable
```

说明 AgnesAI/LiteLLM 这一层内部在把请求转 JSON 时遇到了 multipart 文件对象。

更合理的请求形态应该是 JSON 请求体，把参考图转换成字符串输入，例如：

```json
{
  "model": "agnes-video-v2.0",
  "prompt": "...",
  "seconds": "6",
  "size": "1280x720",
  "extra_body": {
    "image": [
      "data:image/png;base64,..."
    ]
  }
}
```

具体字段是否为 `image`、`input_reference` 或 `extra_body.image`，需要以 AgnesAI 视频接口实际文档和上游验收结果为准。但可以确定的是，当前直接透传 `UploadFile` 不是可用格式。

## 和图生图问题的相似点

之前 AgnesAI 图生图失败，是因为 OpenAI image edits 请求路径和请求格式不能直接转给 AgnesAI。

当时修复方向是：

1. `/v1/images/edits` 映射到 AgnesAI 支持的 `/v1/images/generations`。
2. multipart 图片文件转成 `data:<mime>;base64,...`。
3. 请求改为 JSON。
4. `response_format` 移入 AgnesAI 期望的位置。

本次视频失败与之类似：AgnesAI 视频也需要一个 provider-specific 的转换层，不能完全复用 OpenAI/Sora 的 multipart 转发逻辑。

## 可能的修复方向

后续如果要修代码，建议不要直接改前端优先规避，而是在后端 AgnesAI 视频路径做适配。

推荐方向：

1. 给 AgnesAI 视频创建独立 task adaptor，或在 Sora task adaptor 中为 `ChannelTypeAgnesAI` 单独分支。
2. 对 `multipart/form-data` 请求读取 `input_reference[]` 文件。
3. 把每个文件转成 `data:<mime>;base64,...` 字符串。
4. 构造 `application/json` 请求体发给 AgnesAI，而不是继续转发 multipart。
5. 补 `agnes-video-v2.0` 的模型价格配置，避免每次按异常 fallback 预扣 `$75`。
6. 检查 AgnesAI 任务完成响应中的视频 URL 字段，确保任务轮询成功后 `PrivateData.ResultURL` 能保存真实视频 URL。
7. 检查 `/api/canvas/relay/videos/:task_id/content` 是否能正确获取 AgnesAI 的最终视频内容。

## 需要重点验证的文件

后端：

```text
relay/relay_adaptor.go
relay/relay_task.go
relay/channel/task/sora/adaptor.go
controller/canvas_relay.go
controller/video_proxy.go
service/task_polling.go
setting/ratio_setting/model_ratio.go
```

前端：

```text
/Users/a1/Desktop/my-canvas/web/src/services/api/video.ts
```

## 建议测试用例

如果后续开始修复，建议至少加以下测试：

1. AgnesAI 视频 multipart 请求包含 `input_reference[]` 文件时，后端输出 JSON 请求体。
2. 输出 JSON 中不包含 multipart 文件对象。
3. 文件内容被转换成 `data:image/...;base64,...`。
4. `Content-Type` 被设置为 `application/json`。
5. `model` 使用映射后的 `info.UpstreamModelName`。
6. 无参考图的文生视频仍可正常创建。
7. AgnesAI 任务完成响应能正确解析最终视频 URL。

## 当前状态

本分析文档只记录原因，没有修改视频业务代码。

当前已知根因是：

```text
AgnesAI 视频请求复用了 Sora/OpenAI 的 multipart 透传逻辑，导致参考图文件以 UploadFile 对象形式进入上游；AgnesAI/LiteLLM 不能 JSON 序列化该对象，于是任务创建返回 500。
```

