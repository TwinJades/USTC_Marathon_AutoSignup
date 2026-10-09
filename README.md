# 科大马拉松监测与自动报名程序

在 Windows 上监测[青春科大马拉松报名页面](https://young.ustc.edu.cn/login/sc-wisdom-group-learning/myproject/marathonProject)，按照选定比赛的优先顺序检查名额，取得名额后立即提交参赛资料。完成一场比赛并核实网站记录后，结束全部监测。

程序使用独立 Chromium 浏览器、网站自身的查询机制和网页报名按钮。姓名与学号读取当前账户，参赛校区读取目标比赛。

## 安装

需要 Windows 10/11 和 Python 3.12。在项目目录使用 PowerShell 执行：

```powershell
.\setup.ps1
```

脚本在项目目录创建 `.venv`，安装固定版本依赖，并将 Playwright 官方 Chromium 下载到 `runtime/browsers`。第一次安装时创建私人配置 `config.toml`。重复运行安装脚本保留已有配置。

## 填写资料与登录

编辑 `config.toml` 的 `[profile]`：

| 配置字段 | 填写内容 |
| --- | --- |
| `depart_name` | 学院或部门的规范全称，用于第二步 |
| `user_type` | `学生` 或 `教职工` |
| `sex` | `男` 或 `女` |
| `size` | 网站中的完整衣服尺码 |
| `id_number` | 十八位身份证号 |
| `phone` | 本人十一位手机号码，两个步骤使用同一号码 |
| `emergency_contact` | 紧急联系人的十一位手机号码 |

衣服尺码可填写：`XS(160/84A)`、`S(165/88A)`、`M(170/92A)`、`L(175/96A)`、`XL(180/100A)`、`2XL(185/104A)`、`3XL(190/108A)`。

保存账号和密码：

```powershell
.\run.cmd credentials set
.\run.cmd login
```

密码在终端隐藏输入，保存在当前 Windows 用户的凭据管理器中，服务名称为 `USTC Marathon AutoSignup`。私人 TOML 只保存账号与报名资料。也可以直接运行 `login`，在程序打开的浏览器中本人登录；认证成功后程序保存对应账号。

程序通过 `passport.ustc.edu.cn` 进入 `id.ustc.edu.cn`，识别普通账号密码表单并点击“立即登录”。遇到短信、验证码、设备验证时等待本人完成认证。一次登录流程只自动提交一次密码。

## 选择比赛与开始监测

```powershell
.\run.cmd select
.\run.cmd validate
.\run.cmd check
.\run.cmd watch
```

`select` 使用空格勾选比赛，回车确认，再设置比赛优先顺序。选择结果写入配置顶层的 `targets` 数组；数组顺序就是报名优先顺序。

`validate` 检查本地配置；`check` 登录网站并读取目标比赛状态、表单预填资料和本人报名记录；`watch` 执行真实自动报名。

在 `[monitor]` 中设置 `poll_interval_seconds`，支持正数和小数，默认值为 `30.0`。监测期间修改这个间隔，从下一轮检查生效。修改账号、目标比赛或报名资料后需要重新启动监测。

`headless = true` 使用后台浏览器。需要本人认证时打开登录窗口。设置为 `false` 可持续显示程序自己的浏览器窗口。

## 两个报名步骤

第一步进入选定比赛详情，点击“报名”，按照网站提示进入“报名须知”。程序从须知窗口出现开始等待至少十秒，同时等待“确认报名”按钮可用，再点击按钮。

“报名信息”表单中的“院系”和“班级”保留网站预填内容。程序检查预填内容非空，填写配置中的手机号码，再次检查院系和班级保持原值，然后点击“确定”。字段缺失、预填内容为空或出现未知表单时，程序立即终止。

取得名额后进入第二步，填写用户类型、学院或部门、性别、衣服尺码、身份证号、手机号码和紧急联系人电话。程序核对网站预填的姓名、学号及校区，点击“提交”，随后读取本人参赛资料，逐项核实保存结果。

网站要求取得名额后两天内填写参赛资料。程序连续完成两个步骤。已经取得名额的目标比赛优先处理，避免继续报名其他比赛。

## 状态与停止

```powershell
.\run.cmd status
.\run.cmd credentials status
```

按 `Ctrl+C` 停止检查和后续提交。`status` 显示本地报名阶段：

| 阶段 | 含义 |
| --- | --- |
| `enter_pending` | 第一步已经准备发送请求，结果待核查 |
| `reserved` | 网站报名记录确认取得名额，参赛资料待提交 |
| `save_pending` | 第二步已经准备发送请求，结果待核查 |
| `complete` | 网站报名及参赛资料已经核实 |
| `full` | 网站明确返回名额已满，或提交前检查发现当前无法报名 |

阶段在提交前写入 SQLite。重新启动 `watch` 会读取网站实际记录，再处理未完成报名。提交结果不明确时，程序停止其他比赛报名并保留记录。此前资料提交仍无法确认时，需要在网站核查实际结果；程序终止自动提交。

程序通过 Windows 文件锁限制单实例运行，避免两个进程同时报名。配置错误、未知响应、无法确认的账户或比赛状态会终止程序。全部目标结束报名后自动退出。

## 网站数据依据

查询通过当前网页的 `$http` 实例执行，保留网站既有的认证和参数加密。程序只允许查询已识别的比赛、本人资料、第一阶段选项及本人参与项目接口；真实提交通过网页按钮执行。

人数上限读取详情的 `peopleNum`。人数读取网站用于“项目参与人数”显示的 `registrationNum`。两项字段必须为有效整数，人数上限必须大于零。缺少这些数据时程序终止监测。该人数在实际报名期间是否与即时名额一致，需要登录账户和真实比赛核实，参见 [验证记录](VERIFICATION.md)。

当前实现支持现场、单次马拉松活动，以及第一步包含“院系”“班级”“手机”的表单。网站表单、字段或报名形式变化时会终止并报告原因。

## 私人文件

`config.toml`、`config.toml.pending`、`.venv` 和整个 `runtime` 目录已加入 `.gitignore`。GitHub 源码包含空白配置示例。运行日志只记录比赛编号、公开比赛名称、人数和操作阶段，账号在数据库中保存为 SHA-256 摘要。

`runtime/browser-profile` 含有登录会话，`config.toml` 含有本人资料，均应只保存在本机。浏览器会话与现有选课项目分别保存。

## 验证命令

```powershell
.\.venv\Scripts\python.exe -m unittest discover -s tests -v
.\.venv\Scripts\python.exe -m pip check
.\.venv\Scripts\python.exe tools\verify_public.py
```

本地测试检查真实文件、实际 SQLite 数据库和 Windows 文件锁。公开网页验证使用独立后台浏览器，读取真实认证页面和前端资源，不输入账号资料。当前验证边界记录在 [VERIFICATION.md](VERIFICATION.md)。

认证识别流程参考 [USTC 选课助手](https://github.com/TwinJades/USTC_Automatic_Course_Selection_Tool) 的实现。
