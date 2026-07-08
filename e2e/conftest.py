"""
e2e/conftest.py — pytest fixtures for Playwright browser-based integration tests.
"""
import json
import os
import pytest
from playwright.sync_api import sync_playwright, Browser, Page


BASE_URL = os.environ.get("XWORKBENCH_URL", "http://localhost:8902")


@pytest.fixture(scope="session")
def browser() -> Browser:
    """Launch Chromium (headless) for the entire test session."""
    with sync_playwright() as p:
        browser = p.chromium.launch(
            headless=True,
            args=["--no-sandbox", "--disable-setuid-sandbox"],
        )
        yield browser
        browser.close()


@pytest.fixture(scope="session")
def base_url() -> str:
    return BASE_URL


@pytest.fixture
def page(browser: Browser, base_url: str) -> Page:
    """Open a new page per test, navigate to app, wait for sidebar."""
    ctx = browser.new_context(viewport={"width": 1400, "height": 900}, locale="zh-CN")
    page = ctx.new_page()
    page.goto(base_url, wait_until="load")
    page.wait_for_selector(".sidebar", timeout=10_000)
    yield page
    ctx.close()


class _ApiHelper:
    """
    Thin wrapper around Playwright's APIRequestContext.
    Playwright's page.request does not accept json=; we serialize manually.
    """

    def __init__(self, page: Page, base_url: str):
        self._page = page
        self._base = base_url

    def _do(self, method: str, path: str, body=None, **kwargs):
        """body: dict → JSON bytes; None → no body."""
        kw = dict(kwargs)
        if body is not None:
            import json as _json
            kw["data"] = _json.dumps(body)
            hdrs = dict(kw.get("headers", {}))
            hdrs["Content-Type"] = "application/json"
            kw["headers"] = hdrs
        return getattr(self._page.request, method)(f"{self._base}{path}", **kw)

    def get(self, path: str, **kwargs):
        return self._do("get", path, **kwargs)

    def post(self, path: str, body=None, **kwargs):
        return self._do("post", path, body=body, **kwargs)

    def put(self, path: str, body=None, **kwargs):
        return self._do("put", path, body=body, **kwargs)

    def delete(self, path: str, **kwargs):
        return self._do("delete", path, **kwargs)


@pytest.fixture
def api(page: Page, base_url: str):
    return _ApiHelper(page, base_url)
