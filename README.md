# dsh-pet-mygo

[dsh-pet](https://github.com/PC2005-cloud/dsh-pet) 的无 Electron / 无 DeepSeek Harness
桌面端：用 [MyGo](https://github.com/egoist/mygo) 的透明置顶窗 + 系统 webview（WKWebView）
播放 HEVC-with-alpha `.mov`，对外保持 dsh-pet 自己的 `/dsh-pet-7340` HTTP 契约。

**闸门实验已通过**：MyGo 透明无边框窗能透明渲染 HEVC-alpha mov（无黑框），因此整个
桌宠可以不依赖 116MB 的 Electron 运行。

## 当前状态

MVP 骨架，已跑通闭环：

- 透明、无边框、置顶、无阴影窗口，播放真实宠物动画（蓝毛小女仆）
- **双缓冲 `<video>` 交叉淡入**：切换动画无闪黑
- **拖拽 / 甩抛**：按住拖动窗口跟随，松手速度快则抛掷（反弹+摩擦），慢则原地放下
- **多宠物 / 多窗口**：每只宠物独立窗口、独立状态与气泡，bridge 用 `CallerWindow` 区分窗口
- **屏幕漫游**：idle 一段时间后随机方向行走（探到屏幕边缘不走出），播行走动画 + 窗口滑动，2 秒后回 idle
- `/dsh-pet-7340` 契约端点：`GET /state`、`GET /config`、`POST /anim`、
  `POST /broadcast`、`POST /position`（归一化坐标）、`POST /throw`（抛掷+反弹+摩擦），
  均按 `?pet=` 路由
- 状态结构与 dsh-pet 一致（`pets.<id>.{say,anim} = {counter, data}`），
  因此之前写的 [pi 桥接扩展](../../.pi/agent/extensions/dsh-pet.ts) **零改动**直接可用
- 7 个 HEVC-alpha mov 素材（idle + 6 档工作状态），从 npm 包的 VP9-alpha webm 转出

## 运行

```sh
# 先转换素材（已转好可跳过；需要 macOS + ffmpeg + Xcode CLT）
./scripts/encode-movs.sh 待机呼吸休闲 工作状态-思考冒泡 工作状态-忙碌点按 \
  工作状态-清点归档 工作状态-原地踱步张望 工作状态-雀跃庆祝 工作状态-垂头叹气冒汗

go run .        # 监听 http://127.0.0.1:3080，窗口播放 idle 动画
```

驱动它（与 dsh-pet / pi 桥接扩展同款调用）：

```sh
curl -s -X POST 'http://127.0.0.1:3080/dsh-pet-7340/anim?pet=main' \
  -H 'content-type: application/json' -d '{"name":"工作状态-雀跃庆祝"}'

curl -s -X POST 'http://127.0.0.1:3080/dsh-pet-7340/broadcast?pet=main' \
  -H 'content-type: application/json' -d '{"text":"宿主骨架跑通啦","duration":6000}'
```

## 架构

```
main.go                    透明窗 + /dsh-pet-7340 HTTP 服务 + 内存状态
resources/index.html       <video> 播放 mov，轮询 /state 切换动画 + 气泡
resources/anim/*.mov       HEVC-alpha 透明素材
scripts/encode-movs.sh     webm(VP9-alpha) → mov(HEVC-alpha) 转码
```

- 页面通过 `file://` 加载，`fetch` 到 HTTP 服务属跨域，服务端已加 CORS 头。
- 动画名即 `resources/anim/` 下的文件名（不含扩展名）。

## 素材：webm → HEVC-alpha mov

WKWebView 不认 VP9-alpha（黑底），需要 HEVC-with-alpha。ffmpeg 两条路都走不通
（`libx265` 打不开 `yuva*`，`hevc_videotoolbox` 静默丢 alpha），正确做法是复用
dsh-pet 上游的 AVFoundation 编码器：

```
ffmpeg 解码 webm(VP9-alpha) → BGRA 原始帧 → swift 编码器(AVAssetWriter + hevcWithAlpha) → mov
```

`hevc_alpha_encoder.swift`（dsh-pet 上游，MIT）已放在 `scripts/`，`encode-movs.sh`
会按需用 `xcrun swiftc` 编译它。

## 下一步

- 朝向镜像（朝左时动画水平翻转）
- 补齐契约其余端点（`/playback/{id}`）
- 从 dsh-pet Release `assets-mov` 拉全量真实素材（当前只转了 10 个）

## License

MIT（复用 dsh-pet 素材与 `hevc_alpha_encoder.swift` 时保留其 MIT 声明）。
