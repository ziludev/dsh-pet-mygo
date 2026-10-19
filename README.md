# dsh-pet-mygo

[dsh-pet](https://github.com/PC2005-cloud/dsh-pet) 的无 Electron / 无 DeepSeek Harness
桌面端：用 [MyGo](https://github.com/egoist/mygo) 的透明置顶窗 + 系统 webview（WKWebView）
播放 HEVC-with-alpha `.mov`，对外保持 dsh-pet 自己的 `/dsh-pet-7340` HTTP 契约。

**闸门实验已通过**：MyGo 透明无边框窗能透明渲染 HEVC-alpha mov（无黑框），因此整个
桌宠可以不依赖 116MB 的 Electron 运行。

## 当前状态

已对齐 dsh-pet 原版桌宠核心交互：

- 透明、无边框、置顶、无阴影窗口，播放真实宠物动画（蓝毛小女仆）
- **双缓冲 `<video>` 交叉淡入**：切换动画无闪黑
- **动画链**：idle 播完按权重（idle 10 / turn 5 / move 5 / 动作 80）自动选下一个，
  turn 翻转朝向、move 漫游、动作从 80+ 池随机
- **拖拽 / 甩抛**：按住拖动窗口跟随，松手速度快则抛掷（反弹+摩擦），慢则原地放下
- **点击穿透 + 动态命中**：透明区域不拦截鼠标（不遮挡下层窗口），命中区按当前动画的透明像素
  实时计算（`Pet.SetHitBox`），跳跃/举手等超出默认区域的姿势也能拖、能点
- **点击回应**：点击播 clicks 池动画 + Q 弹挤压（squish）
- **右键菜单**：按分类点播动画、清空气泡、回到初始位置
- **碎碎念**：周期自动弹一句 + 碎碎念动画；无模型时用内置句子池结构化降级
- **屏幕漫游 + 朝向镜像**：朝随机方向行走不走出屏幕，朝左时视频水平翻转
- **多宠物 / 多窗口**：每只宠物独立窗口、独立状态与气泡，bridge 用 `CallerWindow` 区分
- `/dsh-pet-7340` 契约端点：`GET /state`、`GET /config`、`POST /anim`（回 `playbackId`）、
  `GET /playback/{id}`（播放回执）、`POST /playback/{id}/report`、`POST /whisper`、
  `POST /broadcast`、`POST /position`（归一化坐标）、`POST /throw`（抛掷+反弹+摩擦），
  均按 `?pet=` 路由
- 状态结构与 dsh-pet 一致（`pets.<id>.{say,anim,facing} = {counter, data}`），
  因此之前写的 [pi 桥接扩展](../../.pi/agent/extensions/dsh-pet.ts) **零改动**直接可用
- 106 个 HEVC-alpha mov 素材（npm 包 VP9-alpha webm 全量转码）

## 与原版对比

实测数据（macOS arm64，`du` / `ps` / `wc`）：

| 指标 | dsh-pet-mygo | dsh-pet 原版 | 差距 |
|---|---|---|---|
| 分发包体积 | **16MB** 单二进制 | Electron 295MB（解压后）+ npm 包 98MB ≈ **393MB** | 小 96% |
| 运行时内存 | 66MB（实测 RSS） | Electron 典型 150~300MB | 省一半以上 |
| 进程数 | **1** | 宿主 + Electron helper + 每宠物 renderer | 单进程 |
| 代码量 | **1234 行**（Go 857 + 页面 377） | **25656 行** TS（host 13605 / client 5696 / shared 4891 / standalone 1464） | 少 95% |
| 素材体积 | mov 134MB（106 个） | webm 52MB（macOS 另需 mov 82MB） | 本项目更大 |
| 外部依赖 | 无（系统 webview） | Electron 首次下载 116MB | 零下载 |

```
分发包体积
  mygo 16M   ██
  原版 393M  ██████████████████████████████████████████████

代码量
  mygo 1234   ██
  原版 25656  ██████████████████████████████████████████████

进程数
  mygo 1     ██
  原版 多     ██████████
```

**本项目的优势**：零依赖零下载（不用等 Electron 116MB）、单二进制分发、代码量 1/20
（删掉了 DSH 宿主的 15k 行路由/事件/helper 进程管理/多屏坐标）、内存省一半以上。

**原版仍占优的地方**（如实列出）：余额动画 / 对话聊天 / 系统通知 / pet pack
（本项目降级或未做，缺模型与余额源）、素材更小（webm 52MB vs mov 134MB）、
动画链按真实播放时长（本项目用固定 6 秒近似）、多宿主生态与正式发布积累。

一句话：用 1/20 代码、1/25 体积实现约 80% 的核心桌宠体验，剩下的 20% 是依赖
外部凭证的能力与细节打磨。

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

## 配置

配置文件：`~/.config/dsh-pet-mygo/config.json`（`DSH_PET_MYGO_CONFIG` 可覆盖路径）。

首次运行自动生成默认配置；拖动 / 漫游 / 抛掷后位置会自动写回（后台每 2 秒检查一次变化），
重启恢复原位。

```json
{
  "pets": [
    { "id": "main", "x": 700, "y": 220 }
  ]
}
```

- `pets`：每个元素一只宠物、一个独立窗口（`id` 不同即可多开）。
- `x` / `y`：窗口左上角的逻辑屏幕坐标。

> 注：MyGo 创建窗口时的 `X` 在小于约 221 时会被 AppKit clamp，所以本项目在创建后用
> `SetPosition` 显式定位，配置里的任意坐标都能生效。

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

- 可选增强：余额动画（需余额源）、对话聊天（接 pi 或外部模型）、系统通知
- 素材入库策略（当前 106 个 mov 约 134MB，可改用 Git LFS 或脚本生成）

## License

MIT（复用 dsh-pet 素材与 `hevc_alpha_encoder.swift` 时保留其 MIT 声明）。
