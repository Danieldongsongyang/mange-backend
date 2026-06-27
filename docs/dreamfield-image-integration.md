# Dreamfield OpenAI 兼容生图 API 接入说明

本文档记录 `https://www.dreamfield.top` 这个第三方 OpenAI 兼容生图 API 接入当前后端的方案。当前观测到该服务由 OneAPI/New-API 类代理提供，支持 OpenAI 风格的 `/v1/models` 与 `/v1/images/generations`，可用模型为 `gpt-image-2`。

注意：不要把真实上游密钥写入文档、代码、测试用例或提交记录。下文统一使用 `<DREAMFIELD_API_KEY>`、`$DREAMFIELD_API_KEY` 作为占位符；如果真实密钥已经在聊天、日志或截图中暴露，建议在上游后台轮换密钥。

## 接入结论

已新增 `Dreamfield` 渠道类型，但不新增专属 relay adaptor；该渠道类型复用当前项目已有的 OpenAI 兼容协议与 OpenAI adaptor。

原因：

- 上游接口路径与 OpenAI Image API 一致：`/v1/images/generations`。
- 鉴权方式为标准 Bearer Token：`Authorization: Bearer <key>`。
- 返回体包含标准 OpenAI 图像字段：`data[0].b64_json`。
- 当前项目已经在 `router/relay-router.go` 注册了 `POST /v1/images/generations`，并会进入 `types.RelayFormatOpenAIImage`。
- `Dreamfield` 渠道类型会映射到 `APITypeOpenAI`，由 OpenAI adaptor 按当前请求路径拼接上游 Base URL，所以请求会转发到 `https://www.dreamfield.top/v1/images/generations`。

如果后续上游出现非 OpenAI 兼容差异，再考虑新增 `relay/channel/dreamfield` 专属 adaptor。

## 上游能力摘要

| 项目 | 当前值 |
| --- | --- |
| 上游 Base URL | `https://www.dreamfield.top` |
| 模型列表接口 | `GET /v1/models` |
| 生图接口 | `POST /v1/images/generations` |
| 鉴权方式 | `Authorization: Bearer <DREAMFIELD_API_KEY>` |
| 可用模型 | `gpt-image-2` |
| 返回格式 | OpenAI Image API 格式 |
| 图片字段 | `data[].b64_json` |
| 代理特征 | 响应头存在 `X-Oneapi-Request-Id` |

模型列表示例响应：

```json
{
  "data": [
    {
      "id": "gpt-image-2",
      "object": "model",
      "created": 1626777600,
      "owned_by": "custom"
    }
  ],
  "object": "list",
  "success": true
}
```

生图示例请求体：

```json
{
  "model": "gpt-image-2",
  "prompt": "a cute cartoon cat, orange tabby, sitting on a desk",
  "n": 1,
  "size": "1024x1024"
}
```

## 后台渠道配置

在管理后台新增渠道：

| 配置项 | 建议值 |
| --- | --- |
| 类型 | `Dreamfield` |
| 名称 | `Dreamfield Image` |
| Base URL | `https://www.dreamfield.top` |
| 密钥 | `<DREAMFIELD_API_KEY>` |
| 模型 | `gpt-image-2` |
| 测试模型 | `gpt-image-2` |
| 分组 | 按业务需要选择，例如 `default` |
| 渠道额外设置 | 通常留空；如需代理，可按 `docs/channel/other_setting.md` 配置 `proxy` |

如果希望对外暴露原始模型名，模型列表直接填写：

```text
gpt-image-2
```

如果希望对外暴露别名，例如 `dreamfield-image`，则：

```text
模型列表：dreamfield-image
模型映射：{"dreamfield-image":"gpt-image-2"}
```

别名方案适合隐藏上游模型名，或未来替换上游模型时保持客户端调用不变。直接暴露 `gpt-image-2` 更简单，建议一期先使用直接模型名。

## 计费与模型定价

`gpt-image-2` 不是当前项目默认定价中必然存在的模型。新增渠道后，需要在后台的模型/倍率配置里为 `gpt-image-2` 配置价格或倍率，否则用户调用时可能因为模型价格未配置而被拒绝。

建议一期按“每张图片固定价格”理解该模型，并结合上游实际成本配置：

- 若后台支持固定价格，给 `gpt-image-2` 设置单次生图固定价格。
- 若使用倍率模式，给 `gpt-image-2` 设置合理模型倍率，并确认生图请求的 `n` 会被计入生成数量。
- 分组倍率仍按当前项目的分组计费规则生效。

当前项目的图像请求处理会读取 `n`，默认值为 `1`。对于 OpenAI 兼容图像响应，如果上游没有返回 usage，项目会用最小 usage 值完成日志和计费流程，因此更推荐用固定价格或明确倍率来管理成本。

## 客户端调用当前后端

接入后，客户端不需要直接调用 Dreamfield，而是调用当前后端统一 OpenAI 兼容入口：

```bash
export NEW_API_BASE_URL="https://<your-new-api-domain>"
export NEW_API_TOKEN="<NEW_API_TOKEN>"

curl -sS "$NEW_API_BASE_URL/v1/images/generations" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -d '{
    "model": "gpt-image-2",
    "prompt": "a cute cartoon cat, orange tabby, sitting on a desk",
    "n": 1,
    "size": "1024x1024"
  }'
```

预期响应格式：

```json
{
  "created": 1710000000,
  "data": [
    {
      "b64_json": "<base64 png data>"
    }
  ]
}
```

如果使用别名模型，则客户端请求里的 `model` 改为别名：

```json
{
  "model": "dreamfield-image",
  "prompt": "a cute cartoon cat, orange tabby, sitting on a desk",
  "n": 1,
  "size": "1024x1024"
}
```

## 直连上游验证

直连上游只用于排查上游密钥、模型权限和服务可用性，不作为业务调用方式。

### 查询模型

```bash
export DREAMFIELD_BASE_URL="https://www.dreamfield.top"
export DREAMFIELD_API_KEY="<DREAMFIELD_API_KEY>"

curl -sS "$DREAMFIELD_BASE_URL/v1/models" \
  -H "Authorization: Bearer $DREAMFIELD_API_KEY"
```

如果密钥权限正常，应能看到 `gpt-image-2`。

### 生图请求

```bash
curl -sS "$DREAMFIELD_BASE_URL/v1/images/generations" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $DREAMFIELD_API_KEY" \
  -d '{
    "model": "gpt-image-2",
    "prompt": "a cute cartoon cat, orange tabby, sitting on a desk",
    "n": 1,
    "size": "1024x1024"
  }'
```

如返回 `data[0].b64_json`，说明上游生图链路正常。

### 解码保存图片

```python
import base64
import json
import os
import pathlib
import urllib.request

base_url = os.environ.get("DREAMFIELD_BASE_URL", "https://www.dreamfield.top")
api_key = os.environ["DREAMFIELD_API_KEY"]

payload = {
    "model": "gpt-image-2",
    "prompt": "a cute cartoon cat, orange tabby, sitting on a desk",
    "n": 1,
    "size": "1024x1024",
}

request = urllib.request.Request(
    f"{base_url}/v1/images/generations",
    data=json.dumps(payload).encode("utf-8"),
    headers={
        "Content-Type": "application/json",
        "Authorization": f"Bearer {api_key}",
    },
    method="POST",
)

with urllib.request.urlopen(request, timeout=120) as response:
    data = json.loads(response.read().decode("utf-8"))

image_bytes = base64.b64decode(data["data"][0]["b64_json"])
output = pathlib.Path("/tmp/dreamfield-test-image.png")
output.write_bytes(image_bytes)
print(output)
```

## 通过当前后端验证

完成后台渠道配置、模型定价和分组启用后，通过当前后端验证：

```bash
export NEW_API_BASE_URL="http://127.0.0.1:3000"
export NEW_API_TOKEN="<NEW_API_TOKEN>"

curl -sS "$NEW_API_BASE_URL/v1/models" \
  -H "Authorization: Bearer $NEW_API_TOKEN"
```

确认模型列表里存在 `gpt-image-2` 或对外别名。

再调用生图：

```bash
curl -sS "$NEW_API_BASE_URL/v1/images/generations" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -d '{
    "model": "gpt-image-2",
    "prompt": "a cute cartoon cat, orange tabby, sitting on a desk",
    "n": 1,
    "size": "1024x1024"
  }'
```

排查顺序：

1. 直连上游 `/v1/models` 成功，但当前后端 `/v1/models` 看不到模型：检查渠道模型列表、分组、状态和用户 token 可用分组。
2. 当前后端 `/v1/models` 能看到模型，但生图报“模型价格未配置”：配置 `gpt-image-2` 的模型价格或倍率。
3. 生图请求返回上游鉴权错误：检查渠道密钥是否填写正确，是否多了空格或换行。
4. 生图请求返回模型无权限：重新直连上游 `/v1/models` 确认该密钥是否仍能访问 `gpt-image-2`。
5. 生图成功但客户端不能显示图片：确认客户端读取的是 `data[0].b64_json`，并按 base64 解码为图片。

## 当前项目转发链路

请求进入当前后端后的主要链路：

```text
router/relay-router.go
  POST /v1/images/generations
    -> controller.Relay(..., types.RelayFormatOpenAIImage)
      -> relay/helper/valid_request.go
        -> GetAndValidOpenAIImageRequest
      -> relay/image_handler.go
        -> ImageHelper
      -> relay/channel/openai/adaptor.go
        -> GetRequestURL / SetupRequestHeader / ConvertImageRequest
      -> relay/channel/openai/relay_image.go
        -> OpenaiImageHandler
```

重要行为：

- `dto.ImageRequest` 已支持 `model`、`prompt`、`n`、`size`、`quality`、`response_format`、`stream` 等字段。
- `gpt-image-2` 不会触发 `dall-e-2`、`dall-e-3`、`gpt-image-1` 的专属默认参数逻辑。
- OpenAI 渠道会透传标准 JSON 请求体，并把上游响应体原样返回给客户端。
- 如未来需要改动图像请求结构，业务代码中的 JSON 编解码必须使用 `common/json.go` 提供的 `common.Marshal`、`common.Unmarshal` 等封装。

## 可选：新增专属渠道类型

当前已新增专属 `Dreamfield` 渠道类型，并复用 `APITypeOpenAI` 和 OpenAI adaptor。

已实现改动点：

```text
constant/channel.go              # 新增 ChannelTypeDreamfield、默认 Base URL、渠道名称
common/api_type.go               # 将 ChannelTypeDreamfield 映射到 APITypeOpenAI
web/default/src/features/...     # 前端渠道类型、图标、文案
web/default/src/i18n/locales/    # 新增前端翻译
```

除非上游协议出现非 OpenAI 兼容差异，否则仍不需要新增 `relay/channel/dreamfield` adaptor。

## 安全注意事项

- 文档、日志、截图、Issue、PR 描述中不要出现真实上游密钥。
- 上游密钥只配置在渠道密钥字段中，不下发给前端。
- 客户端只使用当前后端签发的 token 调用 `/v1/images/generations`。
- 如果要在本地脚本验证上游，使用环境变量注入密钥，脚本不要提交密钥值。
- 如果上游密钥曾经暴露，优先在上游后台轮换，再更新当前后端渠道配置。

## 后续实现检查清单

- [ ] 新增 Dreamfield 类型渠道，Base URL 为 `https://www.dreamfield.top`。
- [ ] 渠道模型填写 `gpt-image-2`，或配置对外别名和模型映射。
- [ ] 为 `gpt-image-2` 配置模型价格或倍率。
- [ ] 确认渠道分组包含目标用户 token 所在分组。
- [ ] 通过当前后端 `/v1/models` 能看到模型。
- [ ] 通过当前后端 `/v1/images/generations` 能得到 `data[0].b64_json`。
- [ ] 客户端按 `b64_json` 解码显示图片。
