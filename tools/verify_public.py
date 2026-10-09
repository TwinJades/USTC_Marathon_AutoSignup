import asyncio
import hashlib
import json
import os
import sys
from datetime import datetime, timezone
from pathlib import Path
from urllib.parse import urlsplit

from playwright.async_api import async_playwright

ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(ROOT))
from ustc_marathon.browser import MARATHON_URL


async def main():
    runtime = ROOT / "runtime"
    os.environ["PLAYWRIGHT_BROWSERS_PATH"] = str(runtime / "browsers")
    evidence = {"time": datetime.now(timezone.utc).isoformat(), "browser": "isolated-headless-chromium"}
    async with async_playwright() as driver:
        context = await driver.chromium.launch_persistent_context(str(runtime / "public-verification-profile"), headless=True)
        try:
            page = await context.new_page()
            page.set_default_navigation_timeout(30000)
            await page.goto(MARATHON_URL, wait_until="domcontentloaded")
            await page.wait_for_url("https://id.ustc.edu.cn/**", timeout=30000)
            await page.wait_for_function("() => document.querySelector('app-root') !== null", timeout=30000)
            notice = page.get_by_text("知道了", exact=True)
            await notice.wait_for(state="visible", timeout=30000)
            await notice.click()
            await page.wait_for_function("() => Array.from(document.querySelectorAll('input')).some(e => e.type === 'password')", timeout=30000)
            host = urlsplit(page.url).hostname
            evidence["authentication_host"] = host
            evidence["authentication_dom"] = await page.evaluate("""() => Array.from(document.querySelectorAll('input'))
                .filter(e => e.getClientRects().length).map(e => ({type:e.type, name:e.name, id:e.id, placeholder:e.placeholder}))""")
            evidence["authentication_buttons"] = await page.get_by_role("button").all_text_contents()
            evidence["login_probe"] = await page.evaluate(Path(ROOT / "ustc_marathon" / "login_probe.js").read_text(encoding="utf-8"))
            app_response = await context.request.get("https://young.ustc.edu.cn/login/sc-wisdom-group-learning/static/js/app.1a7b0c64.js")
            views_response = await context.request.get("https://young.ustc.edu.cn/login/sc-wisdom-group-learning/static/js/chunk-0d0ec75b.13842608.js")
            if app_response.status != 200 or views_response.status != 200:
                raise RuntimeError("公开前端资源读取失败")
            app = await app_response.text()
            views = await views_response.text()
            for name, source in (("app", app), ("views", views)):
                evidence[name + "_sha256"] = hashlib.sha256(source.encode("utf-8")).hexdigest()
            contracts = {
                "website_request_encryption": "p.encry_http_v1(e)" in app,
                "race_list": "/item/scItem/queryApplyingMarathonItem" in views,
                "participant_count_display": "evaluateList.registrationNum" in views,
                "capacity_display": "e.data.peopleNum" in views,
                "notice_countdown": "this.countdown=10,this.timer()" in views,
                "first_options": "/item/scItemRegistration/getOptions" in views,
                "first_submission": "/item/scItemRegistration/enter/" in views,
                "second_submission": "/item/scItem/marathon/save" in views,
                "own_list": "/item/scParticipateItem/list?queryCatelog=0" in views,
            }
            evidence["public_frontend_contracts"] = contracts
            if not all(contracts.values()):
                raise RuntimeError("公开前端结构与程序依据不一致")
            evidence["authenticated_registration_tests"] = "not_run_credentials_and_profile_not_configured"
            destination = runtime / "verification-public.json"
            destination.write_text(json.dumps(evidence, ensure_ascii=False, indent=2), encoding="utf-8")
            print(json.dumps(evidence, ensure_ascii=False, indent=2))
        finally:
            await context.close()


asyncio.run(main())
