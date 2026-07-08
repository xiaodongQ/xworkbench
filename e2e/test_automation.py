"""
e2e/test_automation.py — scheduled / cron task CRUD and scheduler start/stop.
"""
import time
import pytest


class TestAutomation:
    def test_automation_tab_loads(self, page):
        """Automation tab shows the scheduled task list."""
        page.click('[data-tab="automation"]')
        page.wait_for_selector("#page-automation", timeout=5_000)
        assert page.query_selector("#scheduled-list") is not None

    def test_scheduler_start_and_stop(self, page):
        """Scheduler start/stop buttons work without crash."""
        page.click('[data-tab="automation"]')
        page.wait_for_selector("#page-automation", timeout=5_000)
        start_btn = page.query_selector('[data-sched-action="start"]')
        stop_btn  = page.query_selector('[data-sched-action="stop"]')
        assert start_btn is not None
        assert stop_btn is not None
        start_btn.click()
        page.wait_for_timeout(600)
        stop_btn.click()
        page.wait_for_timeout(600)

    def test_create_scheduled_task(self, page, api):
        """POST /api/scheduled creates a cron task visible in the UI."""
        task_name = f"[E2E] Cron {int(time.time())}"
        resp = api.post("/api/scheduled", body={
            "name": task_name,
            "cron_expr": "0 9 * * *",
            "command_type": "shell",
            "prompt": "echo hello",
            "enabled": True,
        })
        data = resp.json()
        assert "id" in data, f"no id: {data}"

        page.click('[data-tab="automation"]')
        page.evaluate("loadScheduled()")
        page.wait_for_timeout(800)
        list_text = page.inner_text("#scheduled-list")
        assert task_name in list_text, f"'{task_name}' not found in scheduled list"

    def test_delete_scheduled_task(self, page, api):
        """Delete a scheduled task and verify it disappears."""
        task_name = f"[E2E] DelSched {int(time.time())}"
        resp = api.post("/api/scheduled", body={
            "name": task_name,
            "cron_expr": "0 10 * * *",
            "command_type": "shell",
            "prompt": "echo bye",
        })
        data = resp.json()
        sched_id = data["id"]

        del_resp = api.delete(f"/api/scheduled/{sched_id}")
        assert del_resp.ok

        page.click('[data-tab="automation"]')
        page.evaluate("loadScheduled()")
        page.wait_for_timeout(800)
        list_text = page.inner_text("#scheduled-list")
        assert task_name not in list_text
