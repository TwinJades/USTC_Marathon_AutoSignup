import asyncio
import json
import os
import sys
from pathlib import Path
from urllib.parse import urlsplit

from playwright.async_api import async_playwright

ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(ROOT))
from ustc_marathon.browser import MARATHON_URL


def public_url(value):
    url = urlsplit(value)
    return url.scheme + "://" + url.netloc + url.path


async def main():
    os.environ["PLAYWRIGHT_BROWSERS_PATH"] = str(ROOT / "runtime" / "browsers")
    async with async_playwright() as driver:
        browser = await driver.chromium.launch(headless=True)
        try:
            page = await browser.new_page()
            failures = []
            statuses = []
            errors = []
            page.on("requestfailed", lambda request: failures.append({"url": public_url(request.url), "failure": request.failure}))
            page.on("response", lambda response: statuses.append({"url": public_url(response.url), "status": response.status}))
            page.on("pageerror", lambda error: errors.append(str(error)))
            await page.goto(MARATHON_URL, wait_until="domcontentloaded")
            await asyncio.sleep(15)
            result = {"url": public_url(page.url), "title": await page.title(),
                      "dom_text": await page.locator("body").inner_text(),
                      "failures": failures, "statuses": statuses, "javascript_errors": errors}
            (ROOT / "runtime" / "inspection-public.json").write_text(json.dumps(result, ensure_ascii=False, indent=2), encoding="utf-8")
            print(json.dumps(result, ensure_ascii=False, indent=2))
        finally:
            await browser.close()


asyncio.run(main())
