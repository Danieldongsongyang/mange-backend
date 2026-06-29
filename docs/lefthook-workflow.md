# Lefthook 方案说明

本文档说明后端仓库当前的 `Lefthook` 方案，以及三层检查策略：

1. `pre-commit` 轻量检查
2. 手动本地检查
3. CI / 发布前检查

## 1. pre-commit 轻量检查

本仓库的 `Lefthook` 配置位于 [lefthook.yml](/Users/a1/Desktop/mange-backend/lefthook.yml:1)。

根据 Lefthook 官方文档，常见使用方式是：

- 在仓库根目录编写 `lefthook.yml`
- 执行 `lefthook install`
- 由 Lefthook 将 hook 安装到 `.git/hooks/`

参考：

- [Lefthook 官方首页](https://lefthook.dev/)
- [stage_fixed 配置说明](https://lefthook.dev/configuration/stage_fixed/)
- [root / staged_files / filters 相关说明](https://lefthook.dev/examples/filters/)

当前 `pre-commit` 会执行三步：

1. 对暂存区 `.go` 文件执行 `gofmt`
2. 对受影响的 Go package 执行 `go vet`
3. 对受影响的 Go package 执行 `go test`

### 配置结构

- `gofmt` 脚本在 [scripts/lefthook-gofmt.sh](/Users/a1/Desktop/mange-backend/scripts/lefthook-gofmt.sh:1)
- package 级检查脚本在 [scripts/lefthook-go-packages.sh](/Users/a1/Desktop/mange-backend/scripts/lefthook-go-packages.sh:1)

### 设计原因

- `gofmt` 很快，适合每次提交前执行
- `go vet` 能拦下一部分明显的静态问题
- `go test` 只跑受影响 package，避免每次 `commit` 都跑 `go test ./...`
- `stage_fixed: true` 会在 `gofmt` 修复后自动重新加入暂存区

### 安装方式

如果你本机已经安装了 `lefthook`，在仓库根目录执行：

```bash
lefthook install
```

或者直接使用：

```bash
make hooks-install
```

如果你尚未安装 `lefthook`，可以优先用官方支持的方式安装，例如：

```bash
brew install lefthook
```

安装完成后，可以手动验证：

```bash
lefthook run pre-commit
```

或者：

```bash
make hooks-run-pre-commit
```

### 当前还额外配置了 pre-push

除了 `pre-commit`，当前还配置了一个很自然的第二道本地自动检查：

```bash
go test ./...
```

它位于 [lefthook.yml](/Users/a1/Desktop/mange-backend/lefthook.yml:1) 的 `pre-push` 中。  
这样形成的是：

- `pre-commit`：轻量、快速、只看受影响 package
- `pre-push`：更重一点，推送前跑全量 Go 测试

这个分层比“每次 commit 都全量测试”更平衡。

## 2. 手动本地检查

当你改动比较大，或者碰到核心逻辑时，建议手动执行更完整的本地检查。

推荐顺序如下：

1. 全量 Go 测试

```bash
go test ./...
```

2. 全量 Go 静态检查

```bash
go vet ./...
```

3. 后端编译检查

```bash
go build ./...
```

如果你这次改动同时影响前端联调、嵌入资源或发布构建，还建议补充：

1. 默认前端类型检查

```bash
cd web/default
bun run typecheck
```

2. 默认前端构建

```bash
cd web/default
bun run build
```

3. 经典前端构建

```bash
cd web/classic
bun run build
```

适合主动做这层检查的场景：

- 一次改了很多 Go 文件
- 改到了 `controller/`、`service/`、`model/`、`relay/`
- 改到了数据库、计费、权限、路由等核心逻辑
- 你已经不太确定影响范围

## 3. CI / 发布前检查

CI / 发布前检查的目标不是“快速提交”，而是“确保仓库真的可构建、可发布”。

这个仓库现有的相关工作流主要在：

- [release.yml](/Users/a1/Desktop/mange-backend/.github/workflows/release.yml:1)
- [docker-build.yml](/Users/a1/Desktop/mange-backend/.github/workflows/docker-build.yml:1)
- [electron-build.yml](/Users/a1/Desktop/mange-backend/.github/workflows/electron-build.yml:1)

建议这层至少覆盖：

1. 全量 Go 测试

```bash
go test ./...
```

2. 全量 Go 静态检查

```bash
go vet ./...
```

3. Go 构建验证

```bash
go build ./...
```

4. 默认前端构建

```bash
cd web/default
bun run build
```

5. 经典前端构建

```bash
cd web/classic
bun run build
```

6. Docker 构建验证

```bash
docker build -t new-api-local-check .
```

如果以后继续完善 CI，推荐的演进方向是：

- 在 PR 阶段自动跑 `go test ./...`
- 在 PR 阶段自动跑 `go vet ./...`
- 在发布前统一校验 Go 与前端构建
- 在需要时加入更完整的集成测试

## 一句话总结

- `pre-commit`：快，拦低级错误
- `pre-push`：补一层全量 Go 测试
- 手动本地检查：大改动时主动补强
- CI / 发布前检查：确保仓库可构建、可发布
