"""
e2e/test_dir_shortcuts.py — directory shortcut CRUD.
"""
import time
import pytest


class TestDirShortcuts:
    def test_dir_list_in_sidebar(self, page):
        """#dir-list exists in the sidebar."""
        page.wait_for_selector(".sidebar", timeout=5_000)
        el = page.query_selector("#dir-list")
        assert el is not None

    def test_create_local_dir_shortcut(self, page, api):
        """Create a local dir shortcut via API; verify it can be retrieved."""
        name = "[E2E] Local " + str(int(time.time()))
        resp = api.post("/api/dir-shortcuts", body={
            "name": name,
            "type": "local",
            "path": "/tmp/e2e-test-dir",
        })
        data = resp.json()
        assert "id" in data, f"create failed: {data}"

        # Verify via API GET
        list_resp = api.get("/api/dir-shortcuts")
        list_data = list_resp.json()
        ids = [d["id"] for d in list_data]
        assert data["id"] in ids, f"created shortcut {data['id']} not in list"

    def test_delete_dir_shortcut(self, page, api):
        """Delete a dir shortcut; reload page and verify it's gone."""
        name = "[E2E] DelDir " + str(int(time.time()))
        resp = api.post("/api/dir-shortcuts", body={
            "name": name,
            "type": "local",
            "path": "/tmp/e2e-delete",
        })
        data = resp.json()
        shortcut_id = data["id"]

        del_resp = api.delete(f"/api/dir-shortcuts/{shortcut_id}")
        assert del_resp.ok

        page.reload(wait_until="networkidle")
        page.wait_for_selector(".sidebar", timeout=5_000)
        sidebar_text = page.inner_text("#dir-list")
        assert name not in sidebar_text
