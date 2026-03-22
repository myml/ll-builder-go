# AGENTS.md

## 项目概述

ll-builder 是玲珑（Linyaps）应用的构建工具，Golang 重写版本。用于在隔离容器中构建应用，并支持推送和发布流程。

## 项目结构

```
├── cmd/              # CLI 命令定义（cobra）
├── internal/
│   ├── builder/      # 构建逻辑核心
│   ├── config/       # 配置加载
│   ├── container/    # 容器运行时（crun OCI）
│   ├── ostree/       # OSTree 仓库操作
│   ├── repo/         # 远程仓库交互
│   ├── source/       # 源码获取（git）
│   ├── types/        # 类型定义
│   └── utils/        # 工具函数
├── pkg/              # 公共包
├── demo/             # 示例 DTK 应用
└── main.go           # 入口
```

## 构建与测试

```bash
# 编译
go build -o ll-builder .

# 运行测试（完整流程）
./test.sh

# 手动测试 demo
cd demo && ../ll-builder build
```

## 常用命令

```bash
ll-builder create <name>          # 创建项目
ll-builder build                   # 构建
ll-builder build --skip-pull-depend # 跳过依赖拉取
ll-builder export --layer          # 导出 layer
ll-builder run                     # 运行应用
```

## 代码规范

- Go 1.21+
- 使用 cobra 处理 CLI
- 容器操作通过 OCI config.json 配置 crun
- 错误处理：使用 `fmt.Errorf("context: %w", err)` 包装错误

## 注意事项

- 容器使用 crun 运行，配置在 `internal/container/`
- OSTree 仓库路径：`~/.cache/linglong-builder/repo`
- build 容器内的 apt 沙箱通过挂载配置文件禁用（见 `build.go`）
