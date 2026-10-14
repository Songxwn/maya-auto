# Maya Auto

图形化控制 Autodesk Maya 2024 及以上版本。程序通过 Maya 本机命令端口执行建模、材质、特效、解算、灯光和渲染，同一套工具同时提供给界面、MCP 和 NewAPI 模型。

Windows x64 程序由 GitHub Actions 构建。下载 Release 里的 `maya-auto.exe`，不要在本机执行 `go build`。

## 下载

发布页：<https://github.com/Songxwn/maya-auto/releases>

- `maya-auto.exe`：图形界面
- `maya-auto-mcp.exe`：供 Cursor 以 stdio 启动的 MCP 程序

## 连接 Maya

1. 打开 Maya 2024 或更高版本。
2. 在脚本编辑器的 Python 页运行 `scripts/enable_command_port.py`，或使用界面「连接」页里的脚本。脚本只打开 `127.0.0.1:7001`。
3. 启动 `maya-auto.exe`，在连接页点「连接」。

若 Maya 弹出命令端口安全提示，只允许本机。

## NewAPI

在 NewAPI 页保存网关地址和密钥。地址可以是 `https://主机` 或 `https://主机/v1`。

- 「拉取模型」请求 `/v1/models`。
- 「探测」对选中的模型发送一条最短对话，记录可用性、错误分类和延迟。单次最多 60 个，并发 3。
- 「指令」页把自然语言交给所选模型。模型通过工具调用操作当前 Maya 场景。

密钥写在 `%APPDATA%\maya-auto\config.json`，不会进入仓库。

## MCP

界面运行时使用 HTTP：

```json
{
  "mcpServers": {
    "maya-auto": {
      "url": "http://127.0.0.1:47321/mcp"
    }
  }
}
```

界面没开时使用 stdio。若界面已经开着，`maya-auto-mcp.exe` 会把请求转发到界面，避免第二个连接占住 Maya 命令端口。

```json
{
  "mcpServers": {
    "maya-auto": {
      "command": "C:\\path\\maya-auto-mcp.exe"
    }
  }
}
```

工具覆盖场景、选择、多边形基本体和编辑、材质、摄影机、nParticle、nCloth、力场、Nucleus、逐帧解算、烘焙、几何缓存、拍屏、灯光、渲染设置和渲染。`maya_execute` 可直接执行 Python 或 MEL。

## 构建

推送到 `master` 或 `main` 会运行测试并上传 Windows amd64 产物。推送 `v*` 标签时同时发布 Release。Actions 使用 Go 1.24，不使用 Go 1.26。
