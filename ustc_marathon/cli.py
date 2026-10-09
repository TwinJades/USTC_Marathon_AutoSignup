import argparse
import asyncio
import getpass
import sys
import traceback

import questionary
from pydantic import ValidationError

from . import __version__
from .browser import BrowserSession, SERVICE, credential_backend
from .config import Config, read_config, write_setting
from .engine import check, watch
from .errors import Problem
from .state import Journal, single_instance
from .website import Website


def error_hook(error_type, error, trace) -> None:
    if isinstance(error, KeyboardInterrupt):
        print("\n运行已停止；待核查的提交记录已保留。", file=sys.stderr)
    elif isinstance(error, Problem):
        print(str(error), file=sys.stderr)
    elif isinstance(error, ValidationError):
        print("配置或网站数据未通过验证。", file=sys.stderr)
        for detail in error.errors(include_input=False, include_context=False, include_url=False):
            print("字段：" + ".".join(map(str, detail["loc"])) + "；原因：" + detail["type"], file=sys.stderr)
    else:
        print(f"运行终止，错误类型：{error_type.__name__}。", file=sys.stderr)
        for entry in traceback.extract_tb(trace):
            print(f"  {entry.filename}:{entry.lineno} {entry.name}", file=sys.stderr)


async def choose(config: Config, journal: Journal) -> None:
    async with BrowserSession(config) as browser:
        await browser.login()
        site = Website(browser, journal)
        account = await site.account()
        if not config.monitor.username:
            write_setting("monitor", "username", account["username"])
        races = await site.races()
        if not races:
            raise Problem("网站当前没有可选择的马拉松比赛。")
        selected_ids = {target.id for target in config.targets}
        choices = [questionary.Choice(f"{target.name}（{target.campus}）", value=target,
                                     checked=target.id in selected_ids) for target in races]
        selected = await questionary.checkbox("选择需要监测的比赛（空格勾选，回车保存）：", choices=choices).ask_async()
        if selected is None:
            raise Problem("比赛选择已取消。")
        ordered = []
        remaining = list(selected)
        while len(remaining) > 1:
            target = await questionary.select("选择下一优先级的比赛：", choices=[
                questionary.Choice(f"{item.name}（{item.campus}）", value=item) for item in remaining
            ]).ask_async()
            if target is None:
                raise Problem("优先级选择已取消，配置文件保持原有内容。")
            ordered.append(target)
            remaining.remove(target)
        ordered.extend(remaining)
        write_setting("", "targets", [target.model_dump() for target in ordered])
        for index, target in enumerate(ordered, 1):
            print(f"{index}. {target.name}（{target.campus}）")
        print("目标比赛已保存。")


async def execute(command: str, config: Config, journal: Journal) -> None:
    if command == "login":
        async with BrowserSession(config) as browser:
            await browser.login(visible=True)
            account = await Website(browser, journal).account()
            if not config.monitor.username:
                write_setting("monitor", "username", account["username"])
            journal.event("登录验证完成")
    elif command == "select":
        await choose(config, journal)
    elif command == "check":
        await check(config, journal)
    elif command == "watch":
        await watch(config, journal)


def main() -> None:
    sys.stdout.reconfigure(encoding="utf-8")
    sys.stderr.reconfigure(encoding="utf-8")
    sys.excepthook = error_hook
    parser = argparse.ArgumentParser(description="科大马拉松监测与自动报名程序")
    parser.add_argument("--version", action="version", version=__version__)
    subparsers = parser.add_subparsers(dest="command", required=True)
    for command, help_text in (("login", "登录并验证当前账户"), ("select", "勾选比赛并设置优先级"),
                               ("check", "只读检查比赛和报名资料"), ("watch", "监测并自动完成一场比赛报名"),
                               ("status", "查看本地报名阶段记录")):
        subparsers.add_parser(command, help=help_text)
    credential_parser = subparsers.add_parser("credentials", help="管理 Windows 保存的登录密码")
    credential_parser.add_argument("action", choices=("set", "status"))
    subparsers.add_parser("validate", help="只验证本地配置，不访问网站")
    args = parser.parse_args()
    with single_instance():
        if args.command == "status":
            journal = Journal()
            try:
                rows = journal.rows()
                if not rows:
                    print("尚无报名提交记录。")
                for row in rows:
                    print(f"{row['name']}（{row['campus']}）：{row['phase']}；{row['updated']}")
            finally:
                journal.close()
            return
        config = read_config()
        if args.command == "validate":
            print("配置格式有效。")
            config.require_watch()
            print("报名必填资料和目标比赛配置完整。")
            return
        if args.command == "credentials":
            backend = credential_backend()
            if args.action == "status":
                exists = bool(config.monitor.username and backend.get_password(SERVICE, config.monitor.username))
                print("登录密码已保存。" if exists else "尚未保存登录密码。")
                return
            username = input("统一身份认证账号：").strip()
            if not username:
                raise Problem("统一身份认证账号不能为空。")
            password = getpass.getpass("统一身份认证密码（输入内容隐藏）：")
            if not password:
                raise Problem("密码不能为空。")
            backend.set_password(SERVICE, username, password)
            write_setting("monitor", "username", username)
            print("登录密码已保存到 Windows 凭据管理器。")
            return
        journal = Journal()
        try:
            asyncio.run(execute(args.command, config, journal))
        finally:
            journal.close()
