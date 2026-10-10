# 开发与命令行说明

源码、测试和 Go 模块位于 `src/`，构建与维护脚本位于 `scripts/`。使用 Go 1.27.2，固定依赖记录在 `src/go.mod` 与 `src/go.sum`。

PowerShell 7 在项目根目录执行 `./scripts/build.ps1`。脚本验证依赖、运行测试和静态检查，使用 `CGO_ENABLED=0`、`-trimpath` 和 `-ldflags='-s -w'` 构建 Windows x64 程序。运行文件位于 `release/`，ZIP 位于 `release/packages/`。发布文件不包含私人配置、运行数据、开发说明、验收记录及 SHA256 文件。

## 命令行

默认配置为 EXE 所在目录的 `config.toml`，运行数据位于同目录 `runtime/`。显式指定配置：`ustc-marathon.exe --config D:\其他目录\config.toml check`。

| 命令 | 用途 |
| --- | --- |
| `login` | 统一处理浏览器选择、认证、账户验证与凭据保存 |
| `select` | 读取真实比赛并设置优先级 |
| `check` | 只读验证账户、资料与目标比赛 |
| `watch` | 监测与自动报名 |
| `status` | 查看本地提交阶段 |
| `validate` | 检查本地配置完整性 |
| `credentials set` | 使用统一登录流程验证并保存凭据 |
| `credentials status` | 检查凭据是否存在 |
| `browser list/select` | 查看或更换浏览器 |
| `storage/clean/uninstall` | 查看空间、清理缓存或卸载 |

配置编辑使用 TOML 库保留注释。登录成功后更新网站预填学院与性别；报名再次检查页面预填内容与账户资料相符。普通查询使用网页自身的 `$http`，保留网站认证与请求处理。所有报名提交通过网页按钮完成。

SQLite 保存 `enter_pending`、`reserved`、`save_pending`、`complete` 与 `full` 阶段；同一运行目录使用 Windows 文件锁限制单实例。提交发送前持久保存阶段，结果不明确时只读核查网站记录。浏览器会话按 Chrome、Edge 分别保存，缓存清理保护 Cookie、网站存储和报名数据库。

## 实际验证

在 `src/` 运行 `go test ./...`。真实网站测试需要本机私人配置与 Windows 凭据，使用环境变量显式启用：

```powershell
$env:USTC_LIVE_VERIFY = '1'
go test -v -run '^TestLiveBrowsers$' -timeout 25m .
```

`USTC_LIVE_FORM_VERIFY=1` 启用真实第一步表单验证；该项不会点击发送报名请求的“确定”。`USTC_LIVE_WATCH=1` 仅在目标全部满员时运行监测与停止验证。`USTC_RELEASE_VERIFY=1` 启用独立中文目录的真实卸载与测试凭据清除验证。构建后运行 `./scripts/verify-release.ps1` 检查解压内容、命令入口及卸载。

脱敏证据保存在被忽略的 `release/runtime/`，公开验收说明位于 `docs/VERIFICATION.md`。真实报名验收取决于网站名额和本人资料，不以本地测试代替网站结果。

`USTC_LIVE_LOGIN_VERIFY=1` 使用空白独立浏览器目录验证真实自动认证。`USTC_LIVE_TERMINAL_VERIFY=1` 启用 Windows 终端首次登录验收：真实选择浏览器、账号密码各输入一次、自动认证及成功后保存凭据。此项从本机凭据管理器读取本人现有凭据，输入与输出仅保存在内存中，不保存认证文字记录。
