"""
e2e/test_tasks.py — task list, create, toggle done, delete.
"""
import time
import pytest


class TestTasks:
    def test_tasks_tab_loads(self, page):
        """Tasks tab shows the task list container."""
        page.click('[data-tab="tasks"]')
        page.wait_for_selector("#page-tasks", timeout=5_000)
        assert page.query_selector("#task-list") is not None

    def test_create_task(self, page, api):
        """POST /api/tasks creates a task visible in the UI."""
        task_title = f"[E2E] Task {int(time.time())}"
        resp = api.post("/api/tasks", body={
            "title": task_title,
            "type": "manual",
            "priority": 2,
        })
        data = resp.json()
        assert "id" in data, f"no id: {data}"

        page.click('[data-tab="tasks"]')
        page.wait_for_timeout(500)
        page.evaluate("loadTasks()")
        page.wait_for_timeout(800)
        list_text = page.inner_text("#task-list")
        assert task_title in list_text, f"'{task_title}' not found in task list"

    def test_toggle_task_done(self, page, api):
        """Toggle task to archived (done) status via API."""
        task_title = f"[E2E] Toggle {int(time.time())}"
        resp = api.post("/api/tasks", body={
            "title": task_title,
            "type": "manual",
        })
        data = resp.json()
        task_id = data["id"]

        resp2 = api.put(f"/api/tasks/{task_id}/status", body={"status": "archived"})
        data2 = resp2.json()
        assert "error" not in data2, f"toggle failed: {data2}"

    def test_delete_task(self, page, api):
        """Delete a task and verify it's gone from the list."""
        task_title = f"[E2E] Delete {int(time.time())}"
        resp = api.post("/api/tasks", body={
            "title": task_title,
            "type": "manual",
        })
        data = resp.json()
        task_id = data["id"]

        del_resp = api.delete(f"/api/tasks/{task_id}")
        assert del_resp.ok

        page.click('[data-tab="tasks"]')
        page.evaluate("loadTasks()")
        page.wait_for_timeout(800)
        list_text = page.inner_text("#task-list")
        assert task_title not in list_text
