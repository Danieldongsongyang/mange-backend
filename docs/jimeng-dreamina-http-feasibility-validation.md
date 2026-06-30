# Dreamina OAuth HTTP Adaptor 可行性验证

> 本文档只保留与当前目标直接相关的信息：让 `new-api` 后端使用 Dreamina OAuth 登录态直接调用 Dreamina HTTPS API。验证记录只写脱敏后的协议结构、错误码和判断；不记录 token、cookie、签名原文、账号信息、真实 submit_id、真实 logid、签名媒体 URL 或上传临时密钥。

## 当前目标

`/Users/a1/Desktop/my-canvas/` 发来的请求由 `/Users/a1/Desktop/mange-backend/router/api-router.go` 承接后，最终在 `new-api` 后端中通过 Dreamina/即梦 OAuth HTTP adaptor 完成生成任务。

目标形态：

- 后端直接调用 Dreamina 背后的 HTTPS API。
- 后端自己提交任务、保存上游任务 ID、查询结果、代理或缓存最终媒体。
- 不把本地 `dreamina` CLI 当作完整业务执行器。
- 当前只攻一个阻塞点：确认并复刻或绕过 `X-Req-Sign` 的生成/注入机制。

可接受的工程折中：

- 如果 `X-Req-Sign` 无法在 Go 后端中直接复刻，可以引入很窄的本地 sidecar/helper。
- sidecar/helper 只负责生成或注入 `X-Req-Sign`，不负责完整提交、查询、上传和任务状态管理。

## 非目标

以下内容不再作为当前阶段目标：

- 继续证明“CLI 背后是否是 HTTPS API”。这已经确认。
- 把 `dreamina` CLI 包成长期主执行器。
- 先写完整 adaptor 后再回头处理签名。
- 继续扩展无关 CLI 命令、session 管理或完整工作区管理。
- 在文档中保存任何敏感凭证、签名值、真实账号信息或签名媒体 URL。

## 当前结论

主路线可行：

- Dreamina CLI 使用 OAuth Device Flow 登录，不是浏览器 cookie 导入。
- 登录态可用于真实远端授权调用。
- text-to-image 提交、query_result 查询、上传/resource_store 链路均已抓到 HTTPS 协议结构。
- 手写 HTTP 请求在带上有效 `Authorization + X-Req-Sign` 后可以成功调用 Dreamina 接口。
- `new-api` 现有异步任务、上游任务 ID、结果代理和 Canvas relay 结构可以承接该能力。

剩余关键阻塞：

- `X-Req-Sign` 的生成算法或注入机制尚未复刻。
- OAuth refresh 端点和请求体尚未抓取。
- ImageX/TOS 对象上传的完整 SDK 请求尚未明文抓到，但 `get_upload_token -> object upload -> resource_store -> resource_id` 这条业务链路已经确认。
- `video_generate` 请求体尚未抓到。
- `AigcComplianceConfirmationRequired` 的真实 HTTP 响应结构尚未触发。

## 身份与鉴权

已确认：

- CLI 登录机制是 OAuth Device Flow。
- CLI 本地凭证高度可能存储于 macOS Keychain/keyring。
- 业务 API 请求使用：

```http
Authorization: Bearer <redacted>
X-Req-Sign: <redacted>
X-TT-LOGID: <redacted>
```

关键判断：

- `Authorization` 和 `X-Req-Sign` 都是必要鉴权材料。
- 只有 OAuth access token 不足以独立调用业务接口。
- Dreamina 上游对缺失/错误 `Authorization`、缺失/错误 `X-Req-Sign` 均返回同类错误：

```json
{
  "ret": "1015",
  "errmsg": "login error"
}
```

## 已确认 HTTP 协议

### 公共请求特征

Dreamina CLI 业务请求通常带以下 query 参数：

```text
agent_detect=agent:codex
aid=513695
cli_version=<cli version>
from=dreamina_cli
```

提交类请求还会出现：

```text
generate_id=<client uuid>
babi_param=<urlencoded json>
```

`babi_param` 已观察到包含工具入口、场景、tab、编辑类型、enter_from 等埋点/路由信息。当前判断它属于需要保留的请求上下文，但不是本轮重点。

### text2image 提交

接口：

```http
POST https://jimeng.jianying.com/dreamina/cli/v1/image_generate?agent_detect=agent%3Acodex&aid=513695&babi_param=<urlencoded json>&cli_version=<cli version>&from=dreamina_cli&generate_id=<client uuid>
Content-Type: application/json
Authorization: Bearer <redacted>
X-Req-Sign: <redacted>
X-TT-LOGID: <redacted>
```

请求体：

```json
{
  "agent_scene": "workbench",
  "creation_agent_version": "3.0.0",
  "generate_num": 1,
  "generate_type": "text2imageByConfig",
  "prompt": "...",
  "ratio": "1:1",
  "resolution_type": "2k",
  "subject_id": "<submit_id>",
  "submit_id": "<submit_id>",
  "workspace_id": 0
}
```

响应结构：

```json
{
  "ret": "0",
  "msg": "success",
  "msgDetail": "success",
  "logId": "<redacted>",
  "sysTime": "<timestamp>",
  "data": {
    "submit_id": "<submit_id>",
    "history_id": "<history id>",
    "model_key": "high_aes_general_v50",
    "ratio": "1:1",
    "forecast_resolution": {
      "width": 2048,
      "height": 2048
    },
    "pre_gen_item_ids": ["<item id>"],
    "commerce_info": {
      "credit_count": 3,
      "triplets": [
        {
          "resource_type": "aigc",
          "resource_id": "generate_img",
          "benefit_type": "image_basic_v5_2k"
        }
      ]
    },
    "submit_info": {
      "code": 0,
      "msg": ""
    }
  }
}
```

实现判断：

- 后端应生成本地 `submit_id` UUID，并同时写入 `submit_id` 与 `subject_id`。
- 保存 Dreamina 返回的 `data.submit_id`、`history_id`、`model_key`、`forecast_resolution`、`commerce_info.credit_count` 到任务私有数据或 metadata。
- CLI 写入本地 `tasks.db.request.body` 的结构不是真实 HTTP body；后端实现必须以抓包验证到的 lower snake case HTTP body 为准。

### 结果查询

接口：

```http
POST https://jimeng.jianying.com/mweb/v1/get_history_by_ids?agent_detect=agent%3Acodex&aid=513695&cli_version=<cli version>&from=dreamina_cli
Content-Type: application/json
Authorization: Bearer <redacted>
X-Req-Sign: <redacted>
X-TT-LOGID: <redacted>
```

请求体：

```json
{
  "history_ids": null,
  "need_batch": true,
  "submit_ids": ["<submit_id>"]
}
```

成功响应结构：

```json
{
  "ret": "0",
  "errmsg": "success",
  "logid": "<redacted>",
  "systime": "<timestamp>",
  "data": {
    "<submit_id>": {
      "submit_id": "<submit_id>",
      "status": 50,
      "queue_info": {
        "queue_idx": 0,
        "priority": 1,
        "queue_status": 3,
        "queue_length": 0
      },
      "item_list": [
        {
          "image": {
            "large_images": [
              {
                "image_uri": "tos-cn-i-tb4s082cfz/...",
                "image_url": "<signed url redacted>",
                "width": 2048,
                "height": 2048,
                "format": "png",
                "size": 98374
              }
            ]
          },
          "extra": {
            "credits_consume": 3,
            "template_type": "image"
          },
          "gen_result_data": {
            "result_code": 0,
            "result_msg": "Success"
          }
        }
      ],
      "task": {
        "history_id": "<history id>",
        "task_id": "<history id>",
        "status": 50,
        "submit_id": "<submit_id>"
      },
      "workspace_id": 0
    }
  }
}
```

实现判断：

- `QueryResult` 应直接调用 `/mweb/v1/get_history_by_ids`，不要依赖 CLI 本地 `tasks.db`。
- 图片结果优先从 `data[submit_id].item_list[].image.large_images[]` 提取。
- `status=50` 与 CLI `success` 对应。
- 后端响应里应优先返回代理后的媒体 URL，避免直接暴露 Dreamina 短期签名 URL。

### 上传与 resource_store

链路：

```text
get_upload_token -> ImageX/TOS object upload -> resource_store -> resource_id -> image_generate
```

获取上传 token：

```http
POST https://jimeng.jianying.com/mweb/v1/get_upload_token
Content-Type: application/json
Authorization: Bearer <redacted>
X-Req-Sign: <redacted>
X-TT-LOGID: <redacted>
```

请求体：

```json
{
  "scene": 2
}
```

响应 `data` 结构：

```json
{
  "access_key_id": "<redacted>",
  "secret_access_key": "<redacted>",
  "session_token": "<redacted>",
  "region": "cn",
  "space_name": "<redacted>",
  "space_type": 2,
  "upload_domain": "imagex.bytedanceapi.com",
  "current_time": "<timestamp>",
  "expired_time": "<timestamp>"
}
```

资源登记：

```http
POST https://jimeng.jianying.com/dreamina/mcp/v1/resource_store
Content-Type: application/json
Authorization: Bearer <redacted>
X-Req-Sign: <redacted>
X-TT-LOGID: <redacted>
```

请求体：

```json
{
  "resource_items": [
    {
      "resource_type": "image",
      "resource_value": "tos-cn-i-tb4s082cfz/..."
    }
  ]
}
```

响应结构：

```json
{
  "ret": "0",
  "msg": "success",
  "resource_items": [
    {
      "resource_type": "image",
      "resource_value": "tos-cn-i-tb4s082cfz/...",
      "resource_id": "<resource_id>"
    }
  ]
}
```

image2image 提交时使用：

```json
{
  "agent_scene": "workbench",
  "creation_agent_version": "3.0.0",
  "generate_num": 1,
  "generate_type": "editImageByConfig",
  "prompt": "...",
  "ratio": "1:1",
  "resolution_type": "2k",
  "resource_id_list": ["<resource_id>"],
  "subject_id": "<submit_id>",
  "submit_id": "<submit_id>",
  "workspace_id": 0
}
```

实现判断：

- 上传、资源登记、提交三段可以拆成独立接口。
- 第一版可先实现 text-to-image；image-to-image 的接口边界先预留。
- ImageX/TOS object upload 的完整 SDK 请求仍需补抓或用官方/现有 SDK 复刻。

## `X-Req-Sign` 验证结论

### 结构特征

已观察到：

- `X-Req-Sign` 每个 Dreamina CLI 业务请求都会携带。
- 同一账号、同一路径、同一 body 的两次请求，`X-Req-Sign` 不同。
- `GET /dreamina/cli/v1/dreamina_cli_user_info` 无 body 请求也携带 `X-Req-Sign`。
- 脱敏形态：
  - 长度：`96`
  - 字符集：`base64-ish`
  - 无 `.` 分段，不像 JWT。
- CLI 二进制字符串中存在 `authsdk: sign failed` 与 `X-Req-Sign`。

判断：

- 它更像 authsdk/HTTP 客户端层统一注入的鉴权 proof，而不是某个业务 endpoint 自己生成的字段。
- 它不是简单 deterministic `HMAC(path + body)`。

### 必要性

手写 HTTP 变体验证结果：

| 变体 | Dreamina 结果 | 判断 |
| --- | --- | --- |
| 原始 `Authorization` + 原始 `X-Req-Sign` | `ret=0`, `errmsg=success` | 可成功重放 |
| 去掉 `Authorization`，保留 `X-Req-Sign` | `ret=1015`, `errmsg=login error` | `Authorization` 必需 |
| 保留 `Authorization`，去掉 `X-Req-Sign` | `ret=1015`, `errmsg=login error` | `X-Req-Sign` 必需 |
| 保留 `Authorization`，替换假 `X-Req-Sign` | `ret=1015`, `errmsg=login error` | 签名值必须有效 |

判断：

- `Authorization` 与 `X-Req-Sign` 是共同必要的鉴权材料。
- 服务端把缺失/错误 token 与缺失/错误 sign 都折叠成 `login error`。

### 绑定关系

同一枚有效 `X-Req-Sign` 的变体验证：

| 变体 | Dreamina 结果 | 判断 |
| --- | --- | --- |
| 改 `X-TT-LOGID` | `ret=0`, `errmsg=success` | 未观察到绑定 logid |
| 改 body 中的 `submit_ids` 为不存在 ID | `ret=0`, `errmsg=success`, `data` 为空对象 | 未观察到绑定 body 内容 |
| URL 增加额外 query 参数 | `ret=0`, `errmsg=success` | 未观察到绑定完整 query |
| 用查询请求签名访问 `GET /dreamina/cli/v1/dreamina_cli_user_info` | `ret=0`, `msg=success` | 未观察到绑定 method/path/body |
| 用查询请求签名访问 `POST /mweb/v1/get_upload_token` | `ret=0`, `errmsg=success` | 未观察到绑定 method/path/body |

异常点：

- 把第二次 CLI 查询请求的 `X-Req-Sign` 交叉替换到第一次查询请求上，返回 `ret=1015 / login error`。
- 这说明它不是任意可替换的静态 bearer，仍存在未识别的上下文、时间窗口、生成批次或服务端校验条件。

### 有效期与重放

已验证：

- 捕获后立即手写重放：`ret=0`, `errmsg=success`。
- 捕获后约 180 秒再次用同一 `Authorization`、同一 `X-Req-Sign`、同一 URL、同一 body 重放：仍为 `ret=0`, `errmsg=success`。

判断：

- `X-Req-Sign` 至少不是秒级过期。
- 它也不是严格一次性使用。
- 当前未确认最长有效期。

### 当前模型

最符合现有证据的模型：

- `Authorization: Bearer <OAuth access token>` 证明账号登录态。
- `X-Req-Sign` 是 authsdk 注入的有效登录、设备或环境 proof。
- 服务端要求两者同时存在。
- 业务 endpoint 不明显使用该签名校验具体 body、path、method、query 或 logid。
- 签名短期内可重放，但不能确认长期有效期，也不能确认生成算法。

## 后端落点建议

### 1. credential provider

职责：

- 管理 Dreamina OAuth access token、refresh token 和过期时间。
- 请求时注入 `Authorization: Bearer ...`。
- 后续补充 OAuth refresh 流程。

### 2. request signer

职责：

- 为所有 Dreamina 业务请求补 `X-Req-Sign`。
- 第一优先级：静态定位并复刻 CLI/authsdk 的签名生成入口。
- 第二优先级：实现一个很窄的本地 sidecar/helper，只负责签名生成或签名注入。

约束：

- 不把完整生成、查询、上传流程交给 sidecar/helper。
- 后端仍应直接构造并发送 HTTP 请求。

### 3. text2image submitter

职责：

- 生成 `submit_id` UUID。
- 调用 `/dreamina/cli/v1/image_generate`。
- 保存 `data.submit_id`、`history_id`、`model_key`、`forecast_resolution`、`commerce_info.credit_count`。

### 4. result fetcher

职责：

- 调用 `/mweb/v1/get_history_by_ids`。
- 从 `data[submit_id].item_list[].image.large_images[]` 提取结果。
- 映射 Dreamina 状态码到 OpenAI 风格任务状态。
- 返回代理后的媒体 URL。

### 5. resource uploader

职责：

- 调用 `/mweb/v1/get_upload_token`。
- 上传对象到 ImageX/TOS。
- 调用 `/dreamina/mcp/v1/resource_store` 得到 `resource_id`。
- 为 image-to-image、image-to-video 等能力提供资源 ID。

## `new-api` 接入判断

现有结构适合承接：

- Canvas 请求入口已经存在。
- `RelayTaskSubmit` 支持异步任务提交、预扣费、公开 task id、上游 task id。
- `model.Task.PrivateData.UpstreamTaskID` 可保存 Dreamina `submit_id`。
- `RelayTaskFetch` 可基于公开 `task_id` 查本地任务，再由 adaptor 查询 Dreamina 上游。
- `VideoProxy` 或媒体代理可承接最终媒体 URL，避免直接暴露 Dreamina 签名 URL。

第一版范围建议：

- 先实现 text-to-image 的 submit + fetch 闭环。
- 暂不实现完整 workspace/session 管理，默认使用 `workspace_id: 0`。
- 上传链路作为第二步实现，但先预留接口边界。
- 视频链路等拿到 `/dreamina/cli/v1/video_generate` 真实 body 后再接。

## 下一步

只做与 `X-Req-Sign` 相关的工作：

1. 静态定位 CLI 中 `authsdk: sign failed`、`X-Req-Sign`、`authsdk.Authorizer.Inject` 附近的生成入口。
2. 判断签名生成是否能在 Go 后端复刻。
3. 如果不能复刻，设计最小 sidecar/helper：
   - 输入：method、url、headers/body 的必要上下文，或更小的签名请求上下文。
   - 输出：`X-Req-Sign`。
   - 不负责业务提交、查询、上传。
4. 用最小 text-to-image submit + get_history_by_ids fetch 验证端到端 HTTP adaptor。

## 静态定位补充记录（2026-06-30）

本轮只做静态分析和本地算法复刻，不读取或记录 Keychain 中的 OAuth token、refresh token、device private key，不记录任何真实 `X-Req-Sign`。

### CLI 与函数入口

本机 CLI：

```text
/Users/a1/.local/bin/dreamina
```

二进制特征：

- Go Mach-O arm64。
- Go 版本：`go1.26.1`。
- 主模块：`code.byted.org/videocut-aigc/dreamina_cli`。
- 依赖：`code.byted.org/passport/auth_client/go`，版本 `v0.0.0-20260413022607-7a92262407eb`。

静态定位到的关键入口：

- Dreamina wrapper：`code.byted.org/videocut-aigc/dreamina_cli/components/auth.(*sdkAuthorizer).Inject`
- authsdk 注入入口：`code.byted.org/passport/auth_client/go/authsdk.(*Authorizer).Inject`
- signer 入口：`code.byted.org/passport/auth_client/go/signer.(*DefaultSigner).Sign`
- device key 解析：`code.byted.org/passport/auth_client/go/signer.parseDeviceKey`
- nonce 生成：`code.byted.org/passport/auth_client/go/signer.randomNonce`

判断：

- Dreamina CLI 自己的 `sdkAuthorizer.Inject` 只是薄 wrapper。
- 真正签名逻辑在 `passport/auth_client/go/signer.DefaultSigner`。
- `X-Req-Sign` 是 authsdk 统一注入的 proof，不是 Dreamina 业务 endpoint 内部算法。

### Inject 注入字段

`authsdk.(*Authorizer).Inject` 成功签名后会向请求注入：

```http
Authorization: Bearer <oauth access token>
X-Pub-Key: <base64 pkix public key>
X-Req-Sign: <base64 ecdsa-asn1 signature>
X-Req-Ts: <unix seconds>
X-Req-Nonce: <32-char lowercase hex nonce>
```

这补充解释了先前的异常点：上游实际校验的 proof 不只是单独的 `X-Req-Sign`，还包括 `X-Pub-Key`、`X-Req-Ts`、`X-Req-Nonce` 与 OAuth access token 的组合。

### 签名算法结构

device key：

- 算法标识：`ecdsa-p256-sha256`
- public key：PKIX DER 后 base64。
- private key：PKCS#8 DER 后 base64。
- CLI 登录态中的 auth record 持有这组 device key 材料。

nonce：

- 读取 16 字节随机数。
- 转成 32 字符小写 hex。

timestamp：

- 当前 Unix 秒，十进制字符串。

access token 摘要：

```text
base64(sha256(access_token))
```

待签名 payload：

```text
method + "\n" +
escaped_path_or_slash + "\n" +
base64(sha256(access_token)) + "\n" +
unix_seconds + "\n" +
nonce
```

其中：

- `method` 使用请求原始 method，例如 `POST`。
- `escaped_path_or_slash` 来自 Go `url.URL.EscapedPath()`；为空时使用 `/`。
- 不包含 query。
- 不包含 body。
- 不包含 `X-TT-LOGID`。

签名：

```text
digest = sha256(payload)
signature = ecdsa.SignASN1(P-256 private key, digest)
X-Req-Sign = base64(signature)
```

### 后端复刻结论

结论：可以在 Go 后端中复刻签名算法，不需要为纯签名算法引入长期 sidecar。

已新增后端窄包：

```text
pkg/dreamina/authsign
```

能力：

- 生成 `ecdsa-p256-sha256` device key。
- 解析 authsdk 格式的 base64 PKIX/PKCS#8 device key。
- 生成 authsdk 兼容的 `X-Pub-Key`、`X-Req-Sign`、`X-Req-Ts`、`X-Req-Nonce`。
- 将 proof headers 注入 `http.Header`。
- 单元测试用临时 key 与假 token 验证 canonical payload 顺序和 ECDSA ASN.1 签名可验证性。

仍未完成：

- 安全 credential provider：需要从受控配置或专用凭证存储中读取 OAuth access token、refresh token、过期时间和 device key。
- OAuth refresh 请求体和端点仍需按安全约束补抓或复刻。
- 端到端 text-to-image submit + get_history_by_ids fetch 尚未执行，因为该验证需要读取真实本地 OAuth 登录态和 device private key；本轮没有读取或输出这些敏感材料。

最小后续工程路线：

1. 实现 Dreamina credential provider，输入应来自受控 secret，而不是日志、前端请求或普通配置明文。
2. 使用 `pkg/dreamina/authsign` 在 HTTP client 层注入 proof headers。
3. 先用 `/dreamina/cli/v1/dreamina_cli_user_info` 或 `/mweb/v1/get_upload_token` 做无媒体的鉴权 smoke test。
4. 再做最小 text-to-image submit + get_history_by_ids fetch 闭环。

## 安全记录约束

后续继续追加记录时必须遵守：

- 不记录 token、cookie、Keychain 密码值。
- 不记录 `X-Req-Sign` 原文。
- 不记录真实账号信息、余额、用户 ID。
- 不记录真实 submit_id、真实 logid。
- 不记录签名媒体 URL。
- 只记录脱敏结构、错误码、字段名、状态映射和判断。

## HTTP client 骨架推进记录（2026-06-30）

在 `X-Req-Sign` 算法可复刻的基础上，已继续推进到后端 HTTP adaptor 的最小客户端层。

新增包：

```text
pkg/dreamina
```

职责边界：

- `CredentialProvider`：只负责提供 Dreamina OAuth access token 与 authsdk device key。
- `SecretCredentialProvider`：从受控 JSON secret 解析凭证；解析错误不回显原始 secret。
- `Client`：负责构造 Dreamina URL、写入公共 query、marshal JSON body、注入 `Authorization` 与 `X-Req-*` proof headers、发起 HTTP 请求、解析响应。
- `TextToImageRequest` / `SubmitTextToImage`：按已验证的 `/dreamina/cli/v1/image_generate` body 结构提交文生图。
- `FetchHistory`：按已验证的 `/mweb/v1/get_history_by_ids` body 结构查询结果。
- `GetUploadToken`：实现无媒体 smoke test 可用的 `/mweb/v1/get_upload_token` 请求入口。
- `HistoryResponse.LargeImages`：从 `data[submit_id].item_list[].image.large_images[]` 提取图片结果。

已覆盖的协议细节：

- 默认公共 query：
  - `agent_detect=agent:codex`
  - `aid=513695`
  - `from=dreamina_cli`
  - `cli_version` 由调用方显式传入，避免硬编码未知版本。
- 文生图提交会显式发送：
  - `workspace_id: 0`
  - `generate_num: 1`
  - `subject_id == submit_id`
  - `generate_type: text2imageByConfig`
- 查询请求会显式发送：
  - `history_ids: null`
  - `need_batch: true`
  - `submit_ids: [...]`
- 上传 token 请求默认发送：
  - `scene: 2`
- 签名使用 `req.URL.EscapedPath()`，不包含 query 和 body，与静态定位结果一致。

测试方式：

```bash
go test ./pkg/dreamina/...
```

当前结果：

```text
ok github.com/QuantumNous/new-api/pkg/dreamina
ok github.com/QuantumNous/new-api/pkg/dreamina/authsign
```

测试覆盖：

- 假上游服务端解析 `X-Pub-Key`，按 `method + path + token_digest + ts + nonce` 复验 `X-Req-Sign`。
- 验证 `Authorization: Bearer <token>` 与所有 `X-Req-*` 头会被注入。
- 验证 text-to-image body 和 query 结构。
- 验证 `get_history_by_ids` 的 `history_ids: null` 和结果图片提取。
- 验证 `get_upload_token` 的默认 `scene=2`。
- 验证 HTTP 非 2xx 错误不会把响应 body 中的敏感内容拼进错误信息。
- `pkg/dreamina` 没有直接使用 `encoding/json`，marshal/unmarshal 均走 `common.*` 包装。

真实 smoke test 入口：

- 已新增 `TestSmokeUserInfoFromSecretEnv`。
- 默认跳过，不发真实网络请求。
- 只有显式设置 `DREAMINA_SMOKE_SECRET` 时才会调用 `/dreamina/cli/v1/dreamina_cli_user_info`。
- 可选设置 `DREAMINA_SMOKE_CLI_VERSION` 和 `DREAMINA_SMOKE_BASE_URL`。
- 测试不打印 Dreamina 返回的账号数据，不打印 token、device key 或真实 `X-Req-Sign`。

受控 secret JSON 形态：

```json
{
  "access_token": "<redacted>",
  "device_key": {
    "algorithm": "ecdsa-p256-sha256",
    "public_key": "<redacted>",
    "private_key": "<redacted>"
  }
}
```

也支持扁平字段形态：

```json
{
  "access_token": "<redacted>",
  "device_public_key": "<redacted>",
  "device_private_key": "<redacted>"
}
```

该阶段仍未执行，后续已在“最终真实闭环验证记录”中补齐：

- 未读取 Keychain、CLI auth record 或真实 OAuth token。
- 未记录或打印真实 `X-Req-Sign`。
- 未向 Dreamina 真实服务发起 smoke test。
- 未执行真实 text-to-image submit + get_history_by_ids fetch。

该阶段的下一步判断：

- 如果能安全提供一份受控 JSON secret，后端现在已有足够的客户端能力去跑 `/dreamina/cli/v1/dreamina_cli_user_info` 或 `/mweb/v1/get_upload_token` 鉴权 smoke test。
- smoke test 如果返回 `ret=0` 或明确业务错误，即可确认服务端接受 Go 侧生成的 `X-Req-Sign`。
- 如果返回 `ret=1015 / login error`，优先检查 device public key 是否必须与该 access token 的登录态绑定，其次检查 `cli_version`、公共 query、User-Agent 或其他 CLI 请求上下文字段。

## 最终真实闭环验证记录（2026-06-30）

本轮在用户明确授权后，读取本机 Dreamina Keychain 登录态用于最终验证。敏感材料只在进程内使用，未写入文档，未打印 token、device private key、真实 `X-Req-Sign`、真实 submit_id、logid 或结果 URL。

### Keychain auth record 结构

本机 `dreamina` Keychain 密码项格式：

```text
go-keyring-base64:<base64 json payload>
```

base64 解码后是 JSON auth record，脱敏结构为：

```text
client_key
login_expires_at
access_token
refresh_token
token_expires_at
device_key.Algorithm
device_key.PublicKeyBase64
device_key.PrivateKeyBase64
user_info.user_id
```

已让 `pkg/dreamina.SecretCredentialProvider` 支持：

- 受控 JSON secret。
- 扁平 `device_public_key/device_private_key` 字段。
- authsdk 原始 `device_key.Algorithm/PublicKeyBase64/PrivateKeyBase64` 字段。
- `go-keyring-base64:` 原始 Keychain auth record。

### 无媒体鉴权 smoke test

使用 Go 后端 `pkg/dreamina.Client` 调用：

```text
GET /dreamina/cli/v1/dreamina_cli_user_info
```

结果：

```text
PASS
```

结论：

- Go 后端读取真实 OAuth access token 与 device key 后，能生成被 Dreamina 服务端接受的 `Authorization + X-Req-*` proof。
- 这一步已经证明 `X-Req-Sign` 的 Go 复刻可用。

### text2image submit 差异定位

最初 Go client 发起 `POST /dreamina/cli/v1/image_generate` 时返回：

```text
ret=1017
msg=CheckPermission
```

通过临时 MITM 只记录脱敏结构后，对齐到 CLI 成功请求必须包含以下非签名上下文：

query：

```text
agent_detect=agent:codex
aid=513695
cli_version=2a20fff-dirty
from=dreamina_cli
generate_id=<40 chars>
babi_param=<CLI scene JSON>
```

`babi_param` 脱敏后固定结构：

```json
{
  "scene_lv2": "tool_image",
  "tab_name": "cli",
  "edit_type": "cli",
  "enter_from": "cli",
  "tool_id": "tool_image",
  "sub_tool_id": "tool_image",
  "scene_lv1": "cli"
}
```

headers：

```text
appid: 513695
pf: 7
X-TT-LOGID: <33 chars>
Content-Type: application/json
```

body：

```json
{
  "agent_scene": "workbench",
  "creation_agent_version": "3.0.0",
  "generate_num": 1,
  "generate_type": "text2imageByConfig",
  "prompt": "...",
  "ratio": "1:1",
  "resolution_type": "2k",
  "subject_id": "<36-char uuid>",
  "submit_id": "<36-char uuid>",
  "workspace_id": 0
}
```

关键修正：

- `cli_version` 不能用 `version.json` 的 `1.4.10`；真实 HTTP query 使用 CLI 构建短版本 `2a20fff-dirty`。
- `babi_param` 缺失会导致 `CheckPermission`。
- `appid/pf/X-TT-LOGID` 缺失会导致 `CheckPermission`。
- `submit_id/subject_id` 需要使用 36 字符 UUID 形态；`generate_id` 对齐为 40 字符客户端 ID。
- `X-MCP-Trans-Info` 并不是 text2image 成功请求所需 header，已从 client 默认请求中移除。

### 最终 Go 端 submit + fetch 闭环

最终执行：

```bash
DREAMINA_SMOKE_RUN_GENERATION=1 go test ./pkg/dreamina -run TestSmokeTextToImageSubmitFetchFromSecretEnv -count=1 -v
```

结果：

```text
=== RUN   TestSmokeTextToImageSubmitFetchFromSecretEnv
--- PASS: TestSmokeTextToImageSubmitFetchFromSecretEnv (26.07s)
PASS
ok github.com/QuantumNous/new-api/pkg/dreamina 26.524s
```

该测试完成了：

```text
真实 Keychain/OAuth 登录态
-> Go 侧生成 X-Req-Sign
-> POST /dreamina/cli/v1/image_generate
-> 保存返回的 submit_id 到内存
-> POST /mweb/v1/get_history_by_ids
-> 轮询到 status=50
-> 提取到 large_images 结果
```

结论：

- 这条 Dreamina OAuth HTTP adaptor 路线已经通过真实端到端验证。
- 最大阻塞点 `X-Req-Sign` 已攻克。
- 真实服务端接受 Go 后端生成的签名和补齐后的 CLI 请求上下文。
- 后续工作已经不是可行性问题，而是工程化接入：安全凭证存储、refresh、任务模型落库、结果 URL 代理、计费和 Canvas 路由/adaptor 集成。

安全清理：

- 临时 MITM 仅用于脱敏结构抓取。
- 临时 CA 已从当前用户 Keychain 删除。
- 本轮未在文档或测试输出中记录任何真实 token、private key、`X-Req-Sign`、submit_id、logid 或媒体 URL。
