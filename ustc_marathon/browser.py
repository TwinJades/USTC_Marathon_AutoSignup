import asyncio
import os
import time
from pathlib import Path
from urllib.parse import urlsplit

import keyring
from keyring.backends.Windows import WinVaultKeyring
from playwright.async_api import BrowserContext, Page, async_playwright

from .config import Config, RUNTIME
from .errors import AuthenticationRequired, Problem

SITE = "https://young.ustc.edu.cn"
BASE_PATH = "/login/sc-wisdom-group-learning"
MARATHON_URL = SITE + BASE_PATH + "/myproject/marathonProject"
SERVICE = "USTC Marathon AutoSignup"
BRIDGE = Path(__file__).with_name("page_bridge.js").read_text(encoding="utf-8")
LOGIN_PROBE = Path(__file__).with_name("login_probe.js").read_text(encoding="utf-8")


def credential_backend() -> WinVaultKeyring:
    backend = WinVaultKeyring()
    keyring.set_keyring(backend)
    return backend


class BrowserSession:
    def __init__(self, config: Config):
        self.config = config
        self.driver = None
        self.context: BrowserContext | None = None
        self.page: Page | None = None
        self.headless = True

    async def __aenter__(self) -> "BrowserSession":
        os.environ["PLAYWRIGHT_BROWSERS_PATH"] = str(RUNTIME / "browsers")
        self.driver = await async_playwright().start()
        return self

    async def __aexit__(self, *_):
        try:
            if self.context is not None:
                await self.context.close()
        finally:
            await self.driver.stop()

    async def launch(self, headless: bool) -> None:
        if self.context is not None:
            await self.context.close()
        self.headless = headless
        self.context = await self.driver.chromium.launch_persistent_context(
            user_data_dir=str(RUNTIME / "browser-profile"), headless=headless,
            locale="zh-CN", timezone_id="Asia/Shanghai", viewport={"width": 1280, "height": 900},
        )
        self.context.set_default_timeout(self.config.monitor.request_timeout_seconds * 1000)
        self.context.set_default_navigation_timeout(self.config.monitor.request_timeout_seconds * 1000)
        self.page = await self.context.new_page()
        await self.page.goto(MARATHON_URL, wait_until="domcontentloaded")

    async def ready(self) -> bool:
        if urlsplit(self.page.url).hostname != "young.ustc.edu.cn":
            return False
        return await self.page.evaluate("""() => {
            const vm = document.querySelector('#app')?.__vue__;
            return !!(vm?.$http?.get && vm?.$store?.getters?.token
                && vm?.$store?.getters?.userInfo?.username);
        }""")

    async def login(self, visible: bool = False) -> None:
        await self.launch(headless=not visible and self.config.monitor.headless)
        deadline = time.monotonic() + self.config.monitor.login_timeout_seconds
        submitted = False
        manual_message = False
        password = credential_backend().get_password(SERVICE, self.config.monitor.username) if self.config.monitor.username else None
        while time.monotonic() < deadline:
            if await self.ready():
                await self.page.goto(MARATHON_URL, wait_until="domcontentloaded")
                await self.page.wait_for_function("() => !!document.querySelector('#app')?.__vue__?.$http?.get")
                print("统一身份认证成功。")
                return
            host = urlsplit(self.page.url).hostname
            if host not in {"young.ustc.edu.cn", "passport.ustc.edu.cn", "id.ustc.edu.cn"}:
                raise Problem("认证跳转进入未知域名，已停止自动填写。")
            if host in {"passport.ustc.edu.cn", "id.ustc.edu.cn"} and self.headless:
                await self.launch(headless=False)
                continue
            notice = self.page.get_by_text("知道了", exact=True)
            if await notice.count() == 1 and await notice.is_visible():
                await notice.click()
            probe = await self.page.evaluate(LOGIN_PROBE)
            interactive = probe["kind"] == "verification" or not password
            if interactive and self.headless:
                await self.launch(headless=False)
                continue
            if interactive and not manual_message:
                print("请在程序打开的登录窗口完成认证；程序等待正常认证结果。")
                manual_message = True
            if probe["kind"] == "login" and host in {"passport.ustc.edu.cn", "id.ustc.edu.cn"} and password and not submitted:
                await self.page.locator("input").nth(probe["accountIndex"]).fill(self.config.monitor.username)
                await self.page.locator("input").nth(probe["passwordIndex"]).fill(password)
                button = self.page.get_by_role("button", name="立即登录", exact=True)
                if await button.count() != 1:
                    raise Problem("认证页面登录按钮发生变化，已停止自动提交。")
                await button.click()
                submitted = True
            await asyncio.sleep(0.4)
        raise Problem("等待认证超时，请完成登录后再次运行 login。")

    async def get(self, path: str, **params) -> object:
        for attempt in range(2):
            if not await self.ready():
                await self.login(visible=True)
            actual = await self.page.evaluate("() => document.querySelector('#app').__vue__.$store.getters.userInfo.username")
            if self.config.monitor.username and actual != self.config.monitor.username:
                raise Problem("当前登录账户与配置账号不一致。")
            response = await self.page.evaluate(BRIDGE, {
                "path": path, "params": params,
                "timeout": self.config.monitor.request_timeout_seconds * 1000,
            })
            if response["kind"] == "authentication":
                if attempt == 1:
                    raise AuthenticationRequired("重新认证后查询仍然失效。")
                await self.login(visible=True)
                continue
            data = response["data"]
            break
        if not isinstance(data, dict):
            raise Problem("网站返回了未知的查询结果。")
        if data.get("message") == "Token失效，请重新登录" or str(data.get("code")) == "401":
            raise AuthenticationRequired("登录会话已失效，请重新运行 login。")
        if ("success" in data and data["success"] is not True) or ("success" not in data and str(data.get("code")) != "200"):
            raise Problem("网站未成功完成查询，已停止运行。")
        if "result" not in data:
            raise Problem("网站查询结果缺少 result 字段。")
        return data["result"]

    async def route(self, suffix: str) -> None:
        if not await self.ready():
            raise AuthenticationRequired("登录会话已失效。")
        paths = await self.page.evaluate(r"""() => {
            const routes = document.querySelector('#app').__vue__.$store.state.permission.routers;
            if (!Array.isArray(routes)) throw new Error('网站路由数据发生变化');
            const paths = [];
            const visit = (items, prefix) => {
                for (const route of items) {
                    const path = route.path.startsWith('/') ? route.path : prefix.replace(/\/$/, '') + '/' + route.path;
                    paths.push(path);
                    if (route.children) visit(route.children, path);
                }
            };
            visit(routes, '');
            return paths;
        }""")
        matching = [path for path in paths if path.lower().endswith(suffix.lower())]
        if len(matching) != 1:
            raise Problem("网站目标页面路由发生变化。")
        await self.page.goto(SITE + BASE_PATH + matching[0], wait_until="domcontentloaded")
        await self.page.wait_for_function("() => !!document.querySelector('#app')?.__vue__?.$store?.getters?.token")
