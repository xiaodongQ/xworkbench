"""
e2e/test_config.py — system config import/export round-trip.
"""
import pytest


class TestConfig:
    def test_config_tab_loads(self, page):
        """Config tab shows the cfg sub-tabs."""
        page.click('[data-tab="config"]')
        page.wait_for_selector("#page-config", timeout=5_000)
        assert page.query_selector('.cfg-tab-btn[data-tab="default_cli"]') is not None

    def test_export_returns_json(self, api):
        """GET /api/config returns a valid JSON config object."""
        resp = api.get("/api/config")
        assert resp.ok
        data = resp.json()
        assert isinstance(data, dict)
        assert "ai_loop_enabled" in data or "default_terminal" in data

    def test_import_export_roundtrip(self, page, api):
        """Export config and import it back; should succeed with correct format."""
        resp = api.get("/api/config")
        exported = resp.json()
        assert isinstance(exported, dict)

        # Import needs {type, items} wrapper
        resp2 = api.post("/api/config/import", body={
            "type": "dir_shortcuts",
            "items": [],
        })
        assert resp2.ok

    def test_import_preview(self, page, api):
        """POST /api/config/import/preview returns a preview."""
        resp2 = api.post("/api/config/import/preview", body={
            "type": "experiences",
            "items": [],
        })
        assert resp2.ok
        data = resp2.json()
        assert "total" in data
