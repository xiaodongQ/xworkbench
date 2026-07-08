"""
e2e/test_aichat.py — AI Chat tab: loads and handles send without crash.
"""
import pytest


class TestAIChat:
    def test_aichat_tab_loads(self, page):
        """AI Chat tab renders the chat region and input area."""
        page.click('[data-tab="aichat"]')
        page.wait_for_selector("#aichat-root .aichat-chat-region", timeout=5_000)
        assert page.query_selector("#aichat-input") is not None
        assert page.query_selector("#aichat-send-btn") is not None

    def test_send_message_does_not_crash(self, page):
        """Fill and send a message; the page should not crash."""
        page.click('[data-tab="aichat"]')
        page.wait_for_selector("#aichat-input", timeout=5_000)
        page.fill("#aichat-input", "hello e2e test")
        page.click("#aichat-send-btn")
        page.wait_for_timeout(1500)
        # Input should still be present (no crash)
        assert page.query_selector("#aichat-input") is not None

    def test_ai_config_endpoint_reachable(self, api):
        """GET /api/ai/config returns the AI config structure."""
        resp = api.get("/api/ai/config")
        assert resp.ok
        data = resp.json()
        assert isinstance(data, dict)
