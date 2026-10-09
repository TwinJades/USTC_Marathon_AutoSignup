import asyncio
import re
import time
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from enum import Enum

from playwright.async_api import Locator, TimeoutError as PlaywrightTimeoutError

from .browser import BrowserSession, SITE
from .config import Profile, Target, read_config
from .errors import Problem
from .state import Journal

CHINA = timezone(timedelta(hours=8))
LIST_PATH = "/item/scItem/queryApplyingMarathonItem"
OWN_PATH = "/item/scParticipateItem/list?queryCatelog=0"


def required_text(data: dict, key: str) -> str:
    value = data.get(key)
    if not isinstance(value, str) or not value.strip():
        raise Problem(f"网站结果缺少有效字段：{key}。")
    return value.strip()


def required_integer(data: dict, key: str) -> int:
    value = data.get(key)
    if isinstance(value, bool) or not isinstance(value, (str, int)) or re.fullmatch(r"[0-9]+", str(value)) is None:
        raise Problem(f"网站结果缺少有效整数：{key}。")
    return int(value)


def dictionary(value: object) -> dict:
    if not isinstance(value, dict):
        raise Problem("网站结果结构发生变化。")
    return value


def website_time(value: str) -> datetime:
    parsed = datetime.fromisoformat(value)
    if parsed.tzinfo is None:
        return parsed.replace(tzinfo=CHINA)
    return parsed.astimezone(CHINA)


@dataclass(frozen=True)
class Availability:
    target: Target
    detail: dict
    count: int
    capacity: int
    starts: datetime
    ends: datetime

    def phase(self, now: datetime) -> str:
        if now >= self.ends:
            return "ended"
        if now < self.starts:
            return "waiting"
        return "available" if self.count < self.capacity else "full"


class Outcome(Enum):
    ACCEPTED = "accepted"
    FULL = "full"
    UNKNOWN = "unknown"


class Website:
    def __init__(self, browser: BrowserSession, journal: Journal):
        self.browser = browser
        self.journal = journal

    @property
    def page(self):
        return self.browser.page

    async def account(self) -> dict:
        info = dictionary(await self.browser.get("/item/scItem/marathon/myInfo"))
        username = required_text(info, "username")
        required_text(info, "realname")
        expected = self.browser.config.monitor.username
        if expected and username != expected:
            raise Problem("网站返回的学号与配置账号不一致。")
        return info

    async def pages(self, path: str, **params) -> list[dict]:
        records = []
        seen = set()
        page_number = 1
        while True:
            result = dictionary(await self.browser.get(path, pageNo=page_number, pageSize=100, **params))
            total = required_integer(result, "total")
            batch = result.get("records")
            if not isinstance(batch, list):
                raise Problem("网站分页结果缺少 records。")
            for raw in batch:
                row = dictionary(raw)
                identifier = required_text(row, "id")
                if identifier in seen:
                    raise Problem("网站分页出现重复记录，请重新查询。")
                seen.add(identifier)
                records.append(row)
            if len(records) >= total:
                return records
            if not batch:
                raise Problem("网站分页结果不完整。")
            page_number += 1

    async def races(self) -> list[Target]:
        targets = []
        for row in await self.pages(LIST_PATH):
            detail = dictionary(await self.browser.get("/item/scItem/queryById", id=required_text(row, "id")))
            target = Target(id=required_text(detail, "id"), name=required_text(detail, "itemName"),
                            campus=required_text(detail, "marathonCampus"))
            if str(detail.get("marathon")) != "1":
                raise Problem("马拉松列表中出现其他类型项目。")
            targets.append(target)
        return targets

    async def own_records(self) -> dict[str, dict]:
        return {row["id"]: row for row in await self.pages(OWN_PATH)}

    async def details(self, target: Target) -> dict:
        detail = dictionary(await self.browser.get("/item/scItem/queryById", id=target.id))
        actual = Target(id=required_text(detail, "id"), name=required_text(detail, "itemName"),
                        campus=required_text(detail, "marathonCampus"))
        if actual != target or str(detail.get("marathon")) != "1":
            raise Problem("目标比赛名称、编号、校区或类型发生变化，请重新选择。")
        if str(detail.get("itemCategory")) != "0" or str(detail.get("form")) != "0":
            raise Problem("目标比赛的报名形式不属于当前支持的现场单次活动。")
        return detail

    async def availability(self, target: Target) -> Availability:
        detail = await self.details(target)
        capacity = required_integer(detail, "peopleNum")
        if capacity == 0:
            raise Problem("网站人数上限为零，无法可靠判断剩余名额。")
        evaluation = dictionary(await self.browser.get("/lib/scItemProgramme/selectEvaluationByItemId", id=target.id))
        count = required_integer(evaluation, "registrationNum")
        starts = website_time(required_text(detail, "applySt"))
        ends = website_time(required_text(detail, "applyEt"))
        if starts >= ends:
            raise Problem("网站报名起止时间不正确。")
        return Availability(target, detail, count, capacity, starts, ends)

    async def first_options(self, target: Target) -> dict[str, dict]:
        result = await self.browser.get("/item/scItemRegistration/getOptions", itemId=target.id)
        if not isinstance(result, list):
            raise Problem("网站第一阶段表单结构发生变化。")
        options = {}
        for raw in result:
            item = dictionary(raw)
            label = required_text(item, "text")
            if item.get("value") == "picUrls":
                continue
            if label in options:
                raise Problem("第一阶段表单字段重复。")
            options[label] = item
        if set(options) != {"院系", "班级", "手机"} or options["手机"].get("value") != "phone":
            raise Problem("第一阶段报名字段发生变化，需要核查真实表单。")
        for label in ("院系", "班级"):
            required_text(options[label], "text_value")
        return options

    async def form_item(self, dialog: Locator, label: str) -> Locator:
        pattern = re.compile(r"^\s*" + re.escape(label) + r"\s*[:：]?\s*$")
        item = dialog.locator(".ant-form-item").filter(has=self.page.locator("label").filter(has_text=pattern))
        if await item.count() != 1:
            raise Problem(f"报名页面无法唯一识别字段：{label}。")
        return item

    async def text_input(self, dialog: Locator, label: str) -> Locator:
        item = await self.form_item(dialog, label)
        inputs = item.locator("input:not([type=hidden])")
        if await inputs.count() != 1:
            raise Problem(f"报名页面输入字段发生变化：{label}。")
        return inputs

    async def select_value(self, dialog: Locator, label: str, value: str) -> None:
        item = await self.form_item(dialog, label)
        await item.locator(".ant-select-selection").click()
        dropdown = self.page.locator(".ant-select-dropdown:visible")
        option = dropdown.get_by_text(value, exact=True)
        if await option.count() != 1:
            raise Problem(f"网站缺少配置中的选项：{label}。")
        await option.click()

    async def search_row(self, target: Target, participation: bool = False) -> Locator:
        await self.browser.route("/myproject/HaveSign" if participation else "/myproject/marathonProject")
        field = self.page.get_by_placeholder("请输入项目名称" if participation else "项目名称", exact=True)
        await field.fill(target.name)
        path = OWN_PATH if participation else LIST_PATH
        async with self.page.expect_response(lambda response: path in response.url and response.request.method == "GET"):
            await self.page.get_by_role("button", name="查询", exact=True).click()
        await self.page.locator(".ant-spin-spinning").wait_for(state="hidden")
        row = self.page.locator(f'tr.ant-table-row[data-row-key="{target.id}"]')
        if await row.count() != 1:
            raise Problem("页面中无法唯一定位目标比赛。")
        row_id = await row.get_attribute("data-row-key")
        if row_id != target.id:
            raise Problem("页面比赛编号与目标编号不一致。")
        return row

    async def submission(self, button: Locator, path: str, username: str, target: Target, stage: str) -> Outcome:
        current = read_config()
        expected = self.browser.config
        if current.profile != expected.profile or current.targets != expected.targets or current.monitor.username != expected.monitor.username:
            raise Problem("提交前报名配置发生变化，已停止后续提交。")
        if not await self.browser.ready():
            raise Problem("提交前登录会话已经失效，请重新启动并核查记录。")
        actual = await self.page.evaluate("() => document.querySelector('#app').__vue__.$store.getters.userInfo.username")
        if actual != username:
            raise Problem("提交前当前登录账户发生变化。")
        self.journal.record(username, target, stage)
        # 超时表示提交结果需要只读核查，保留提交前已经写入的记录。
        try:
            async with self.page.expect_response(lambda response: response.url.startswith(SITE + "/login/wisdom-group-learning-bg" + path)
                                                 and response.request.method == "POST") as response_info:
                await button.click()
            response = await response_info.value
        except PlaywrightTimeoutError:
            return Outcome.UNKNOWN
        if response.status != 200:
            return Outcome.UNKNOWN
        data = dictionary(await response.json())
        if data.get("success") is True:
            return Outcome.ACCEPTED
        message = data.get("message")
        if isinstance(message, str) and re.search(r"名额已满|人数已满|报名已满|人数已达上限|报名人数超过|报名人数已达", message):
            return Outcome.FULL
        return Outcome.UNKNOWN

    async def enter(self, target: Target, profile: Profile, username: str) -> Outcome:
        options = await self.first_options(target)
        fresh = await self.availability(target)
        if fresh.phase(datetime.now(CHINA)) != "available":
            return Outcome.FULL
        row = await self.search_row(target)
        await row.locator(".pjList").click()
        detail = self.page.get_by_role("dialog").filter(has=self.page.get_by_text("项目详情", exact=True))
        await detail.wait_for(state="visible")
        await detail.get_by_role("button", name="报名", exact=True).click()
        tips = self.page.get_by_role("dialog").filter(has=self.page.get_by_text("报名须知", exact=True))
        popover = self.page.locator(".ant-popover:visible")
        await tips.or_(popover).first.wait_for(state="visible")
        if await popover.is_visible():
            text = await popover.inner_text()
            if "确定报名" not in text:
                raise Problem("网站出现未知报名确认提示。")
            await popover.get_by_role("button", name="确定", exact=True).click()
        await tips.wait_for(state="visible")
        visible_at = time.monotonic()
        await asyncio.sleep(10.0)
        confirm = tips.get_by_role("button", name="确认报名", exact=True)
        await confirm.wait_for(state="visible")
        if not await confirm.is_enabled() or time.monotonic() - visible_at < 10.0:
            raise Problem("报名须知倒计时或确认按钮状态异常。")
        self.journal.event("报名须知等待完成", seconds=round(time.monotonic() - visible_at, 3), item=target.id)
        await confirm.click()
        dialog = self.page.get_by_role("dialog").filter(has=self.page.get_by_text("报名信息", exact=True))
        await dialog.wait_for(state="visible")
        await self.page.locator(".ant-spin-spinning").wait_for(state="hidden")
        original = {}
        for label in ("院系", "班级"):
            value = await (await self.text_input(dialog, label)).input_value()
            if not value.strip() or value.strip() != required_text(options[label], "text_value"):
                raise Problem("系统预填的院系或班级内容无效。")
            original[label] = value
        await (await self.text_input(dialog, "手机")).fill(profile.phone)
        for label, value in original.items():
            if await (await self.text_input(dialog, label)).input_value() != value:
                raise Problem("系统预填内容发生变化，已停止提交。")
        if await (await self.text_input(dialog, "手机")).input_value() != profile.phone:
            raise Problem("第一阶段手机号码填写结果不一致。")
        latest = await self.availability(target)
        if latest.phase(datetime.now(CHINA)) != "available":
            return Outcome.FULL
        self.journal.event("第一阶段字段检查通过", item=target.id)
        return await self.submission(dialog.get_by_role("button", name="确定", exact=True),
                                     "/item/scItemRegistration/enter/" + target.id, username, target, "enter_pending")

    async def save(self, target: Target, profile: Profile, account: dict) -> Outcome:
        dialog = self.page.get_by_role("dialog").filter(has=self.page.get_by_text("新增", exact=True))
        if not await dialog.is_visible():
            row = await self.search_row(target, participation=True)
            await row.get_by_text("填写信息", exact=True).click()
        await dialog.wait_for(state="visible")
        await self.page.locator(".ant-spin-spinning").wait_for(state="hidden")
        if await (await self.text_input(dialog, "姓名")).input_value() != required_text(account, "realname"):
            raise Problem("第二阶段预填姓名与当前账户不一致。")
        if await (await self.text_input(dialog, "学/工号")).input_value() != required_text(account, "username"):
            raise Problem("第二阶段预填学号与当前账户不一致。")
        campus_item = await self.form_item(dialog, "参赛校区")
        if (await campus_item.locator(".ant-select-selection-selected-value").inner_text()).strip() != target.campus:
            raise Problem("第二阶段参赛校区与目标比赛不一致。")
        for label, value in (("用户类型", profile.user_type), ("性别", profile.sex), ("衣服尺码", profile.size)):
            await self.select_value(dialog, label, value)
        for label, value in (("学院/部门", profile.depart_name), ("身份证号", profile.id_number),
                             ("手机号码", profile.phone), ("紧急联系人联系方式", profile.emergency_contact)):
            await (await self.text_input(dialog, label)).fill(value)
            if await (await self.text_input(dialog, label)).input_value() != value:
                raise Problem(f"第二阶段字段填写结果不一致：{label}。")
        return await self.submission(dialog.get_by_role("button", name="提交", exact=True),
                                     "/item/scItem/marathon/save", account["username"], target, "save_pending")

    async def verified_saved(self, target: Target, profile: Profile, account: dict, row: dict) -> bool:
        status = required_integer(row, "marathonDataStatus")
        if status == 0:
            return False
        if status not in {1, 2}:
            raise Problem("网站返回未知的参赛资料状态。")
        info = dictionary(await self.browser.get("/item/scItem/marathon/my/" + target.id))
        expected = {"username": account["username"], "realname": account["realname"], "itemId": target.id,
                    "campus": target.campus, "type": profile.user_type, "departName": profile.depart_name,
                    "sex": profile.sex, "size": profile.size, "idNumber": profile.id_number,
                    "phone": profile.phone, "emergencyContact": profile.emergency_contact}
        for key, value in expected.items():
            if info.get(key) != value:
                raise Problem(f"网站已保存资料与当前配置不一致：{key}。")
        return True
