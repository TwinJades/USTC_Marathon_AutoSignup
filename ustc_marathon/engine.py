import asyncio
from datetime import datetime

from .browser import BrowserSession
from .config import Config, Target, read_config
from .errors import Problem
from .state import Journal, account_key
from .website import CHINA, Outcome, Website, required_integer


def registered(row: dict | None) -> bool:
    if row is None:
        return False
    if str(row.get("examineStatus")) == "-3":
        raise Problem("目标比赛已被网站终止。")
    status = required_integer(row, "applyStatus")
    if status == 28:
        return True
    if status == 26:
        return False
    raise Problem("目标比赛的本人报名状态需要核查。")


async def confirm_record(site: Website, target: Target) -> dict | None:
    for attempt in range(3):
        row = (await site.own_records()).get(target.id)
        if registered(row):
            return row
        if attempt < 2:
            await asyncio.sleep(1)
    return None


async def complete_reserved(site: Website, target: Target, config: Config, account: dict,
                            row: dict, pending_phase: str = "reserved") -> None:
    await site.availability(target)
    if await site.verified_saved(target, config.profile, account, row):
        site.journal.record(account["username"], target, "complete")
        print(f"报名及资料保存已核实：{target.name}（{target.campus}）。全部监测已经停止。")
        return
    if pending_phase == "save_pending":
        raise Problem("此前资料提交结果仍未确认，请在网站核查；程序保留记录并停止自动提交。")
    site.journal.record(account["username"], target, "reserved")
    outcome = await site.save(target, config.profile, account)
    if outcome == Outcome.FULL:
        raise Problem("取得名额后资料提交返回异常结果，已保留提交记录。")
    saved_row = await confirm_record(site, target)
    for attempt in range(3):
        if saved_row is not None and await site.verified_saved(target, config.profile, account, saved_row):
            site.journal.record(account["username"], target, "complete")
            print(f"报名及资料保存已核实：{target.name}（{target.campus}）。全部监测已经停止。")
            return
        if attempt < 2:
            await asyncio.sleep(1)
            saved_row = (await site.own_records()).get(target.id)
    raise Problem("资料提交结果无法确认，已保留提交记录并停止全部监测。")


async def check(config: Config, journal: Journal) -> None:
    config.require_watch()
    async with BrowserSession(config) as browser:
        await browser.login()
        site = Website(browser, journal)
        account = await site.account()
        records = await site.own_records()
        for index, target in enumerate(config.targets, 1):
            state = await site.availability(target)
            if registered(records.get(target.id)):
                saved = await site.verified_saved(target, config.profile, account, records[target.id])
                label = "报名和资料已完成" if saved else "已经取得名额，资料待填写"
            else:
                await site.first_options(target)
                label = {"ended": "报名已结束", "waiting": "尚未开始报名", "full": "人数已满", "available": "存在空余名额"}[state.phase(datetime.now(CHINA))]
            print(f"{index}. {target.name}（{target.campus}）：{state.count}/{state.capacity}，{label}")
            journal.event("只读检查完成", item=target.id, count=state.count, capacity=state.capacity)
        for row in journal.unresolved(account["username"]):
            print(f"存在待核查记录：{row['name']}，阶段 {row['phase']}。")


async def watch(config: Config, journal: Journal) -> None:
    config.require_watch()
    async with BrowserSession(config) as browser:
        await browser.login()
        site = Website(browser, journal)
        account = await site.account()
        username = account["username"]
        records = await site.own_records()
        unresolved = journal.unresolved(username)
        if unresolved:
            pending = unresolved[0]
            targets = [target for target in config.targets if target.id == pending["item"]]
            if len(targets) != 1:
                raise Problem("未完成报名的比赛已从配置中移除，请核查待处理记录。")
            target = targets[0]
            if target.name != pending["name"] or target.campus != pending["campus"]:
                raise Problem("未完成报名记录与当前目标比赛不一致。")
            row = await confirm_record(site, target)
            if row is None:
                raise Problem("此前提交结果仍无法确认，已停止全部监测；程序不会重复发送报名请求。")
            await complete_reserved(site, target, config, account, row, pending["phase"])
            return
        completed = {row["item"] for row in journal.rows() if row["account"] == account_key(username) and row["phase"] == "complete"}
        for target in config.targets:
            row = records.get(target.id)
            if registered(row):
                await complete_reserved(site, target, config, account, row)
                return
            if target.id in completed:
                raise Problem("本地完成记录与网站报名记录不一致，请核查实际报名状态。")
        print("开始监测；按 Ctrl+C 停止后续检查和提交。")
        journal.event("监测开始", targets=len(config.targets), interval=config.monitor.poll_interval_seconds)
        while True:
            current = read_config()
            if current.profile != config.profile or current.targets != config.targets or current.monitor.username != config.monitor.username:
                raise Problem("报名资料、目标比赛或账号在监测期间发生变化，请重新启动监测。")
            current.profile.require_complete()
            records = await site.own_records()
            ended = 0
            for target in config.targets:
                row = records.get(target.id)
                if registered(row):
                    await complete_reserved(site, target, config, account, row)
                    return
                state = await site.availability(target)
                phase = state.phase(datetime.now(CHINA))
                print(f"{datetime.now(CHINA):%H:%M:%S} {target.name}：{state.count}/{state.capacity}，"
                      + {"ended": "报名结束", "waiting": "等待开始", "full": "人数已满", "available": "发现空余名额"}[phase])
                if phase == "ended":
                    ended += 1
                    continue
                if phase != "available":
                    continue
                outcome = await site.enter(target, config.profile, username)
                if outcome == Outcome.FULL:
                    journal.record(username, target, "full")
                    continue
                row = await confirm_record(site, target)
                if row is None:
                    raise Problem("报名提交结果无法确认，已保留记录并停止全部监测。")
                await complete_reserved(site, target, config, account, row)
                return
            if ended == len(config.targets):
                print("全部目标比赛已经结束报名，监测已停止。")
                journal.event("全部目标报名结束")
                return
            await asyncio.sleep(current.monitor.poll_interval_seconds)
