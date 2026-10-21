# DoMiSo Universal

[English](README.md) · [DoMiSo 语法](SYNTAX_cn.md)

使用 mygo 原生 GPU 界面的 Go 桌面 MIDI 编辑器，无需 WebView、Node 或 Wails。支持 Windows 与 Linux；游戏监听和自动演奏仅支持 Windows。

## 使用

- 打开 TXT/DMS/JSON/MIDI，或选择内置曲谱。文本通过“从文本重新生成 MIDI”显式生成；编辑后 MIDI 为唯一音乐数据，文字保留为参考。
- 选择轨道，双击空白创建音符，拖动移动、拖右边缘改变时长，空白拖动框选，Shift 多选，右击删除。支持多轨、静音、独奏、音量、撤销重做。
- Ctrl+C/X/V 复制、剪切、粘贴，Delete 删除，方向键移调，Ctrl+Z/Y 撤销重做。空格播放/暂停，Esc 停止。先点卷帘或键盘区域，再使用电脑键盘弹奏/录音。
- 滚轮垂直移动，Shift+滚轮水平移动，Ctrl+滚轮缩放。拖动标尺定位，Ctrl/Alt 设置循环起止。播放模式禁用音符编辑，拖动平移并定位。
- 标签可切换音名、带八度点的简谱和映射按键；简谱主音值采用 MIDI 音高，C4=60。负数预滚区不能创建或录制音符。

## 音源与保存

每轨可选择 SoundFont SF2、单个 WAV/OGG/MP3 或 JSON 多采样包。单采样需填写原始 MIDI 音高；变调通过改变采样播放速度完成。默认自然衰减，“随音符结束”在松键时淡出。不加载音源也能用内置合成音试听。

多采样包格式见 [sample-pack.example.json](sample-pack.example.json)。JSON 与采样放在同一目录，`rootKey` 不重复，路径不能超出包目录。优先使用相同音高，否则选择最近采样，距离相同时选择较低原始音高。首版不含 SFZ、力度分层或循环点编辑。

可选的 `soundfonts/GeneralUser-GS.sf2` 提供默认 GM 音色；仓库不分发 SoundFont。音源文件最大 256MiB，已解码音源预算 512MiB。音频缓冲可选 20/40/80ms，重启后生效，实际延迟由设备决定。

项目保存为 `曲谱.json` 和同名 `曲谱.assets/` 文件夹，音源会复制到文件夹内。搬移或分享时保留两者。支持旧 JSON v1/v2，新增保存格式为 v3。MIDI 导出保留音乐事件，但不携带自定义音源设置。已编辑 MIDI 不能保存为纯文本。

## 练习和游戏演奏

内置原神风物之诗琴 21 键映射：Q–U、A–J、Z–M。练习要求按键音高集合精确匹配，并至少有一个进入目标和弦后的新按键；等音模式暂停至正确输入，手动暂停不会被按键恢复。

Windows 游戏自动演奏：准备时间线，切到白名单游戏后，Ctrl+Alt+F8 开始/停止，Ctrl+Alt+F9 紧急停止。失去游戏前台时停止并释放注入按键。游戏全局监听不阻断实体按键、不产生本地声音。

自定义映射可导入/导出 JSON。旧 WebView 版本的配置请先导出 JSON，再导入原生版。

本版精简了 Signal 的参数曲线、外部 USB/蓝牙 MIDI 设备连接和音频导出。

## 构建

安装 Go 1.27.1：

```sh
go run .
go test ./...
go vet ./...
go build -trimpath -o bin/ ./...
```

也可使用 `task dev`、`task build`、`task test`、`task package`。Linux 需要 GTK 3、OpenGL/Mesa，以及 PulseAudio/PipeWire 或 ALSA 运行库，不需要 WebKit 或 CGo。Windows/Linux 的 GitHub Actions 会运行测试并生成构建产物。

旧 Signal 前端可以从 `feature/unified-signal-editor` 分支提交 `812e44c` 恢复，MIT 许可证保留在 `LICENSES/Signal-MIT.txt`。本地旧工作区和源码压缩包为忽略文件，不提交音源或构建结果。

## 原神特别版规约

任何使用本版本软件（原神特别版）创作的衍生作品均需要在作品中注明，并在文字描述部分恰当地注明软件来源。任何由于使用本软件对第三方所造成的侵害均由使用者本人负责。
