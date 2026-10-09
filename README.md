# 科大马拉松监测与自动报名程序

在 Windows 上监测科大马拉松比赛，按配置顺序检查报名状态，取得名额后立即填写参赛资料。完成一场比赛并核实网站记录后结束监测。

## 下载与使用

从 [GitHub Releases](https://github.com/TwinJades/USTC_Marathon_AutoSignup/releases) 下载 Windows x64 ZIP，解压到可写目录。本机需要安装 Chrome 或 Edge。

在解压目录打开 PowerShell，创建私人配置：

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

查询通过网页自己的 `$http` 实例完成，保留原有认证和参数加密；提交通过正常网页按钮完成。人数上限读取 `peopleNum`，当前报名人数读取详情的 `applyNum`。验收同时核对网站参与人数接口的 `registrationNum` 和真实详情页面。每轮核对目标仍在当前账户的可报名列表中。缺少可靠数据、未知字段结构或无法核实的状态会终止程序。实际验证范围见 [VERIFICATION.md](VERIFICATION.md)。

每次提交前将阶段持久保存到 SQLite：

| 阶段 | 含义 |
| --- | --- |
| `enter_pending` | 第一步准备发送，结果待核查 |
| `reserved` | 网站确认取得名额，资料待填写 |
| `save_pending` | 第二步准备发送，结果待核查 |
| `complete` | 报名与资料均已核实 |
| `full` | 当前没有可用名额 |

重新启动先核查未完成记录，优先完成已取得名额的比赛。提交超时或结果不明确时停止其他比赛报名，仅查询实际记录；仍无法核实则退出。服务器明确返回名额已满时继续监测。全部目标结束报名时退出。

`Ctrl+C` 停止后续检查与提交，保留待核查记录。Windows 文件锁限制同一项目单实例运行。

## 私人文件

私人配置和整个 `runtime` 目录通过 `.gitignore` 排除上传。`runtime/sessions/chrome` 和 `runtime/sessions/edge` 保存程序的独立登录会话。网站资源缓存禁用，磁盘缓存限制为 1 MiB；浏览器仍会维护自身所需的会话文件。

`runtime/state.sqlite3` 保存报名阶段，账户使用 SHA256 摘要；`runtime/activity.jsonl` 保存脱敏操作记录。上传问题报告时请保留个人资料与认证信息的隐私。

## 开发与构建

使用 Go 1.27.2。模块版本记录在 `go.mod` 和 `go.sum`。PowerShell 7 执行：

```powershell
.\build.ps1
```

脚本验证依赖、执行本地测试与静态检查，使用 `CGO_ENABLED=0` 构建 EXE，并生成 ZIP 与 SHA256。构建产物在 `dist`，运行无需安装 Go。

真实网站只读验收使用本机配置和凭据：

```powershell
$env:USTC_LIVE_VERIFY = '1'
go test -v -run '^TestLiveBrowsers$' -timeout 25m .
```

脱敏结果保存在 `runtime/verification-go-chrome.json`、`runtime/verification-go-edge.json`。登录实现参考科大选课助手的正常统一认证流程。
