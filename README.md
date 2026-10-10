# 科大马拉松监测与自动报名程序

在 Windows 上监测科大马拉松比赛，按配置顺序检查报名状态，取得名额后立即填写参赛资料。完成一场比赛并核实网站记录后结束监测。

## 下载与使用

从 [GitHub Releases](https://github.com/TwinJades/USTC_Marathon_AutoSignup/releases) 下载 Windows x64 ZIP，解压到可写目录。本机需要安装 Chrome 或 Edge。

双击 `ustc-marathon.exe` 或 `run.cmd` 进入中文终端菜单。首次使用选择“创建或修改配置”，填写账号与报名资料，再保存登录凭据、选择浏览器和目标比赛。

菜单提供登录、资料设置、比赛选择、检查、监测、状态、空间查看、缓存清理和卸载。使用方向键和回车操作；兼容终端以数字选择。操作结束或发生错误后，窗口保留结果，按回车返回菜单。

也可以在解压目录打开 PowerShell，使用命令行：

```powershell
Copy-Item config.example.toml config.toml
.\ustc-marathon.exe credentials set
.\ustc-marathon.exe login
```

已有 `config.toml` 时直接使用。首次执行需要浏览器的命令时，程序列出已安装的 Chrome 和 Edge，由本人选择并保存。也可以执行：

```powershell
.\ustc-marathon.exe browser list
.\ustc-marathon.exe browser select
```

`[browser]` 的 `kind` 留空表示尚未选择，选择后为 `chrome` 或 `edge`。`executable_path` 可填写非标准安装位置的浏览器 EXE 完整路径。无法找到指定浏览器时程序退出。

## 报名资料

编辑私人配置的 `[profile]`：

| 字段 | 填写内容 |
| --- | --- |
| `depart_name` | 学院或部门规范全称，第二步使用 |
| `user_type` | `学生` 或 `教职工` |
| `sex` | `男` 或 `女` |
| `size` | 网站提供的完整衣服尺码 |
| `id_number` | 十八位身份证号 |
| `phone` | 本人十一位手机号码，两个报名步骤共用 |
| `emergency_contact` | 紧急联系人十一位手机号码 |

尺码：`XS(160/84A)`、`S(165/88A)`、`M(170/92A)`、`L(175/96A)`、`XL(180/100A)`、`2XL(185/104A)`、`3XL(190/108A)`。

姓名与学号读取登录账户，参赛校区读取目标比赛。密码隐藏输入，保存在当前 Windows 用户的凭据管理器中。程序兼容原有 `USTC Marathon AutoSignup` 凭据与 SQLite 报名记录。

## 选择与监测

```powershell
.\ustc-marathon.exe select
.\ustc-marathon.exe validate
.\ustc-marathon.exe check
.\ustc-marathon.exe watch
.\ustc-marathon.exe status
```

`select` 读取真实列表，通过空格勾选比赛，再选择优先顺序，保存到 `targets`。`validate` 检查本地配置；`check` 只读查询；`watch` 执行真实自动报名；`status` 查看本地提交阶段。

`poll_interval_seconds` 默认为 `30.0`，支持有限正数与小数。监测期间修改间隔，从下一轮生效。账号、目标、报名资料或浏览器改变时程序终止，需要重新启动。

默认 `headless = true` 后台监测。登录窗口支持普通账户密码自动认证；短信、验证码和设备验证等待本人完成。

## 报名与结果核查

第一步点击“报名”，报名须知出现后等待至少十秒，同时等待“确认报名”按钮可用。随后保留“院系”和“班级”的系统预填内容，填写配置中的“手机”，点击“确定”。预填必填内容为空时立即终止。

取得名额后立即完成第二步。程序核对预填姓名、学号和校区，填写参赛资料，点击“提交”，随后查询本人记录逐项核对。网站要求取得名额后两天内填写资料。

查询通过网页自己的 `$http` 实例完成，保留原有认证和参数加密；提交通过正常网页按钮完成。人数上限读取 `peopleNum`，当前报名人数读取详情的 `applyNum`。验收同时核对网站参与人数接口的 `registrationNum` 和真实详情页面。每轮核对目标仍在当前账户的可报名列表中。缺少可靠数据、未知字段结构或无法核实的状态会终止程序。实际验证范围见发布包内 `VERIFICATION.md`，源码目录中的对应文档为 `docs/VERIFICATION.md`。

每次提交前将阶段持久保存到 SQLite：

| 阶段 | 含义 |
| --- | --- |
| `enter_pending` | 第一步准备发送，结果待核查 |
| `reserved` | 网站确认取得名额，资料待填写 |
| `save_pending` | 第二步准备发送，结果待核查 |
| `complete` | 报名与资料均已核实 |
| `full` | 当前没有可用名额 |

重新启动先核查未完成记录，优先完成已取得名额的比赛。提交超时或结果不明确时停止其他比赛报名，仅查询实际记录；仍无法核实则退出。服务器明确返回名额已满时继续监测。全部目标结束报名时退出。

`Ctrl+C` 停止后续检查与提交，保留待核查记录。菜单中的当前操作停止后返回菜单，命令行操作停止后退出。Windows 文件锁限制同一项目单实例运行。

## 私人文件

程序默认使用 EXE 同目录的 `config.toml` 和 `runtime`，从其他工作目录启动时保持相同位置。`--config` 可指定其他配置文件，运行数据保存到该配置同目录的 `runtime`。

私人配置和整个 `runtime` 目录通过 `.gitignore` 排除上传。`runtime/sessions/chrome` 和 `runtime/sessions/edge` 保存程序的独立登录会话。浏览器启动参数禁用非必要扩展、同步和模型下载；浏览器退出后，将明确可重新生成的缓存、模型、统计和历史文件移入回收站，保留 Cookie、网站存储及认证设置。

`runtime/state.sqlite3` 保存报名阶段，账户使用 SHA256 摘要；`runtime/activity.jsonl` 保存脱敏操作记录。上传问题报告时请保留个人资料与认证信息的隐私。

## 空间查看、清理与卸载

```powershell
.\ustc-marathon.exe storage
.\ustc-marathon.exe clean
.\ustc-marathon.exe uninstall
```

`storage` 显示实际占用分类；`clean` 清理独立浏览器的可再生成文件，并保留配置、认证和报名记录。

`uninstall` 执行全部清除：清除当前账户的本项目 Windows 凭据，等待程序退出后，将整个发布运行目录及其中的配置、会话、日志和报名记录移入回收站。卸载脚本显示移入回收站的大小和磁盘可用空间变化。文件在回收站内仍占用空间，清空回收站后释放对应空间。

卸载要求使用同目录配置，并核查发布目录标识、单实例锁及链接。源码目录、Git 仓库、系统浏览器和 Go 开发环境保留。其他实例运行时卸载终止；卸载执行期间禁止启动新操作。卸载因文件占用中断时，排除占用后可重新执行 `uninstall`。

## 开发与构建

源码与测试位于 `src/`，构建和维护脚本位于 `scripts/`，验收文档位于 `docs/`。使用 Go 1.27.2，模块版本记录在 `src/go.mod` 和 `src/go.sum`。PowerShell 7 在项目根目录执行：

```powershell
.\scripts\build.ps1
```

脚本验证依赖、执行本地测试与静态检查，使用 `CGO_ENABLED=0` 构建 EXE。运行文件位于 `release/`，ZIP 与 SHA256 位于 `release/packages/`，私人配置与会话排除打包。直接运行 EXE 无需安装 Go。

真实网站只读验收使用本机配置和凭据：

```powershell
$env:USTC_LIVE_VERIFY = '1'
Set-Location src
go test -v -run '^TestLiveBrowsers$' -timeout 25m .
```

脱敏结果保存在 `release/runtime/verification-go-chrome.json`、`release/runtime/verification-go-edge.json`。构建后设置 `USTC_RELEASE_VERIFY=1` 并运行 `TestReleaseUninstall`，在独立中文目录验证真实发布程序卸载和测试凭据清除。登录实现参考科大选课助手的正常统一认证流程。
