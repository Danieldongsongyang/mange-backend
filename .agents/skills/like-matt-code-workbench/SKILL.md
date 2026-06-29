---
name: like-matt-code-workbench
description: 在任意仓库中导入 Matt Pocock 的 engineering 和 productivity skills，初始化 Sandcastle，并改成使用第三方 Codex 模型。
disable-model-invocation: true
---

# Like Matt Code Workbench

把当前仓库做成一个可用 Matt Pocock skills 和 Sandcastle 的工作台。

做三件事：

- 导入 `mattpocock/skills` 的 `engineering` 和 `productivity`
- 初始化 Sandcastle
- 把 `.sandcastle` 改成走独立 `CODEX_HOME` 和第三方模型

这是引导式 skill。命令有交互，或者需要用户提供 secret，就停下来让用户做，不要假装已经完成。

## 步骤

### 1. 导入 skills

把这两个目录导入当前仓库的 `.agents/skills/`：

- `https://github.com/mattpocock/skills/tree/main/skills/engineering`
- `https://github.com/mattpocock/skills/tree/main/skills/productivity`

推荐做法是临时克隆 `https://github.com/mattpocock/skills`，只复制这两个目录下的 skill。

不要静默覆盖同名 skill：

- 看起来只是上次导入的旧版本，可以说明后覆盖
- 看起来有用户本地修改，先停下来确认

完成标准：目标 skill 已导入，且没有静默覆盖用户修改。

### 2. 安装 Sandcastle

如果仓库里还没有 `@ai-hero/sandcastle`，就明确提示用户亲自执行：

```bash
npm install --save-dev @ai-hero/sandcastle
```

已经安装就复用，不重复装。

完成标准：依赖已存在。

### 3. 初始化 Sandcastle

明确提示用户亲自执行：

```bash
npx @ai-hero/sandcastle init
```

执行完会生成 `.sandcastle/`。在用户明确说“跑完了”之前，不要继续。

完成标准：用户确认初始化已完成，`.sandcastle/` 已存在。

### 4. 补 `.sandcastle/.env`

如果 `.sandcastle/.env` 不存在而 `.sandcastle/.env.example` 存在，先执行：

```bash
cp .sandcastle/.env.example .sandcastle/.env
```

然后补齐或更新：

```dotenv
CODEX_HOME=/home/agent/workspace/.sandcastle/codex-home
My_Localhost_API_Key=你的第三方APIKey
GH_TOKEN=你的GitHubToken
```

### 5. 写独立 Codex home

创建 `.sandcastle/codex-home/config.toml`：

```toml
model_provider = "mylocalhost"
model = "gpt-5.4"

[model_providers.mylocalhost]
name = "mylocalhost API"
base_url = "http://host.docker.internal:57927/v1"
wire_api = "responses"
env_key = "My_Localhost_API_Key"

[projects."/home/agent/workspace"]
trust_level = "trusted"
```

完成标准：`config.toml` 已存在，且 `env_key` 与 `.env` 一致。

### 6. 改 `.sandcastle/main.ts`

检查 `.sandcastle/main.ts`。

如果整个流程只有一个 Codex 阶段，并且里面有：

```ts
agent: sandcastle.codex("gpt-5.4")
```

就保持 `gpt-5.4` 不变，不需要改模型名。

如果流程分多个阶段，例如 `Implementation` 和 `Review`，就不要把两个阶段都写死成同一个 `sandcastle.codex("...")`。把阶段模型和努力程度提到文件顶部，再在各自阶段分别引用。

通用改法：

```ts
const DEFAULT_CODEX_MODEL = "gpt-5.4";
const IMPLEMENT_CODEX_MODEL = "gpt-5.4";
const REVIEW_CODEX_MODEL = "gpt-5.5";
const IMPLEMENT_CODEX_EFFORT = "medium";
const REVIEW_CODEX_EFFORT = "medium";
```

如果项目里已经有类似 `codexAgent(model, effort)` 的辅助函数，就直接复用它；没有的话，按项目现有写法最小修改。

`Implementation` 阶段改成：

```ts
agent: codexAgent(IMPLEMENT_CODEX_MODEL, IMPLEMENT_CODEX_EFFORT)
```

`Review` 阶段改成：

```ts
agent: codexAgent(REVIEW_CODEX_MODEL, REVIEW_CODEX_EFFORT)
```

如果项目没有 `codexAgent` 辅助函数，而是直接写 `sandcastle.codex(...)`，也按同样思路拆开：

```ts
agent: sandcastle.codex(IMPLEMENT_CODEX_MODEL, {
  effort: IMPLEMENT_CODEX_EFFORT,
})
```

```ts
agent: sandcastle.codex(REVIEW_CODEX_MODEL, {
  effort: REVIEW_CODEX_EFFORT,
})
```

你这个场景里，目标是：

- `Implementation` 使用 `gpt-5.4`
- `Implementation` 的努力程度使用 `medium`
- `Review` 使用 `gpt-5.5`
- `Review` 的努力程度使用 `medium`

不要把“不同阶段的模型和 effort”塞进 `config.toml`。`config.toml` 只负责默认 provider 和默认模型；真正按阶段切换模型与 effort，要在 `.sandcastle/main.ts` 里分别给不同的 `sandbox.run(...)` 传不同的 `agent`。

### 7. 核对

至少核对：

- Matt 的 skills 已导入 `.agents/skills/`
- `.sandcastle/.env` 已包含 `CODEX_HOME`、`My_Localhost_API_Key`、`GH_TOKEN`
- `.sandcastle/codex-home/config.toml` 已存在
- `.sandcastle/main.ts` 的 `Implementation` 阶段使用 `gpt-5.4` 且 effort 为 `medium`
- `.sandcastle/main.ts` 的 `Review` 阶段使用 `gpt-5.5` 且 effort 为 `medium`
- `CODEX_HOME` 是 `/home/agent/workspace/.sandcastle/codex-home`
