# ll-builder (Golang 版本)

这是 ll-builder 的 Golang 重写版本，提供了与 C++ 版本相同的功能。

## 项目结构

```
golang/ll-builder/
├── main.go                    # 程序入口
├── go.mod                     # Go 模块定义
├── cmd/                       # 命令行接口
│   ├── root.go               # 根命令
│   ├── create.go             # 创建项目
│   ├── build.go              # 构建项目
│   ├── run.go                # 运行应用
│   ├── export.go             # 导出应用
│   ├── push.go               # 推送应用
│   ├── list.go               # 列出应用
│   ├── remove.go             # 移除应用
│   ├── import.go             # 导入层文件
│   ├── extract.go            # 提取层文件
│   └── repo.go               # 仓库管理
└── internal/                  # 内部包
    ├── types/                # 数据类型定义
    │   └── types.go
    ├── config/               # 配置文件解析
    │   └── config.go
    ├── builder/              # 核心构建器
    │   └── builder.go
    ├── source/               # 源码获取器
    │   └── source.go
    ├── repo/                 # OSTree 仓库操作
    │   └── repo.go
    └── layer/                # 层文件打包器
        └── layer.go
```

## 编译

```bash
cd golang/ll-builder
go build -o ll-builder
```

## 使用方法

### 创建项目

```bash
./ll-builder create org.deepin.demo
```

### 构建项目

```bash
# 基本构建
./ll-builder build

# 指定配置文件
./ll-builder build -f /path/to/linglong.yaml

# 离线构建
./ll-builder build --offline

# 跳过源码获取
./ll-builder build --skip-fetch-source

# 跳过依赖拉取
./ll-builder build --skip-pull-depend

# 网络隔离
./ll-builder build --isolate-network
```

### 运行应用

```bash
# 运行应用
./ll-builder run

# 调试模式
./ll-builder run --debug

# 指定工作目录
./ll-builder run --workdir /path/to/workdir

# 指定模块
./ll-builder run --modules binary,develop
```

### 导出应用

```bash
# 导出为 UAB (默认)
./ll-builder export

# 导出为层文件 (已弃用)
./ll-builder export --layer

# 指定压缩器
./ll-builder export --compressor zstd

# 指定输出文件
./ll-builder export -o myapp.uab

# 导出指定模块
./ll-builder export --modules binary,develop
```

### 推送应用

```bash
# 推送到默认仓库
./ll-builder push

# 推送到指定仓库
./ll-builder push --repo-url https://repo.example.com --repo-name myrepo

# 推送单个模块
./ll-builder push --module binary
```

### 列出应用

```bash
./ll-builder list
```

### 移除应用

```bash
# 移除指定应用
./ll-builder remove org.deepin.demo

# 移除多个应用
./ll-builder remove org.deepin.demo org.deepin.calculator
```

### 导入层文件

```bash
# 导入层文件
./ll-builder import /path/to/package.layer

# 导入层目录
./ll-builder import-dir /path/to/layer/dir
```

### 提取层文件

```bash
./ll-builder extract /path/to/package.layer /path/to/output
```

### 仓库管理

```bash
# 显示仓库信息
./ll-builder repo show

# 添加仓库
./ll-builder repo add myrepo https://repo.example.com

# 添加仓库带别名
./ll-builder repo add myrepo https://repo.example.com --alias myalias

# 移除仓库
./ll-builder repo remove myalias

# 更新仓库
./ll-builder repo update myalias https://new-repo.example.com

# 设置默认仓库
./ll-builder repo set-default myalias

# 启用镜像
./ll-builder repo enable-mirror myalias

# 禁用镜像
./ll-builder repo disable-mirror myalias
```

## 配置文件

### linglong.yaml

```yaml
version: '1'
package:
  id: org.deepin.demo
  name: demo
  version: 1.0.0.0
  kind: app
  description: Demo application
command: [/opt/apps/org.deepin.demo/files/bin/demo]
base: org.deepin.base/23.0.0
sources:
  - kind: archive
    url: https://example.com/source.tar.gz
    digest: sha256:...
build: |
  # 构建脚本
  cmake -B build
  cmake --build build
  cmake --install build --prefix=$PREFIX
```

## 环境变量

- `LINGLONG_FETCH_CACHE`: 源码缓存目录
- `LINGLONG_OCI_RUNTIME`: OCI 运行时路径
- `LINGLONG_UAB_DEBUG`: UAB 调试模式

## 与 C++ 版本的差异

1. **容器运行**: Go 版本使用 crun 作为 OCI 运行时，支持完整的容器隔离
2. **OSTree 仓库**: Go 版本使用简化的文件系统实现，生产环境需要集成真正的 OSTree
3. **UAB 打包**: Go 版本是模拟打包，生产环境需要集成真正的 UAB 打包器

## 系统依赖

- `ostree` - 用于从仓库拉取 base 和 runtime 依赖
  - Fedora/RHEL: `sudo dnf install ostree`
  - Ubuntu/Debian: `sudo apt install ostree`
  - 如果未安装，将使用 minimal layer（功能受限）
- `crun` - OCI 容器运行时（可选，用于真正的容器隔离）

## Go 依赖

- github.com/spf13/cobra - 命令行解析
- gopkg.in/yaml.v3 - YAML 解析
- github.com/klauspost/compress - 压缩库
- github.com/klauspost/pgzip - Gzip 压缩
- github.com/pierrec/lz4/v4 - LZ4 压缩

## 默认仓库

- URL: `https://mirror-repo-linglong.deepin.com/`
- OSTree 路径格式: `channel/id/version/arch/module`
