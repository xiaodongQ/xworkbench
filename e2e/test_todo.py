"""
e2e/test_todo.py — todo list: add, toggle done, delete.
Uses page.evaluate (browser context) so config state is shared.
"""
import time
import tempfile
import pytest


class TestTodo:
    def test_todo_list_in_tasks_tab(self, page):
        """Todo list container is rendered in the tasks tab."""
        page.click('[data-tab="tasks"]')
        page.wait_for_selector("#page-tasks", timeout=5000)
        assert page.query_selector("#todo-container") is not None

    def _api_do(self, page, method, path, body=None):
        """Call API via page.evaluate (browser fetch), return parsed response."""
        import json
        if body is not None:
            body_str = json.dumps(body)
        else:
            body_str = ""
        return page.evaluate(
            "(async () => {"
            "const opts = {method:'" + method + "',"
            "headers:{'Content-Type':'application/json'}"
            + (",body:'" + body_str.replace("'", "\\'") + "'" if body_str else "") + "};"
            "const r = await fetch('" + path + "', opts);"
            "const text = await r.text();"
            "try { return {status:r.status, data:JSON.parse(text)}; }"
            "catch { return {status:r.status, data:text}; }"
            "})()"
        )

    def test_add_todo_item(self, page):
        """Add a todo item and verify it appears in UI after page reload."""
        path = tempfile.mktemp(suffix=".md")

        # Set path and trigger widget load
        result = self._api_do(page, "PUT", "/api/todo/path", {"path": path})
        assert result["status"] == 200, f"set path failed: {result}"
        # Wait for server config to persist
        page.wait_for_timeout(300)
        # Reload page so widget re-initializes with the new path
        page.reload(wait_until="domcontentloaded")
        page.wait_for_timeout(500)

        content = "[E2E] Todo " + str(int(time.time()))
        post_result = self._api_do(page, "POST", "/api/todo", {"text": content})
        assert post_result["status"] == 200, f"post failed: {post_result}"
        assert "line_no" in post_result["data"], f"no line_no: {post_result}"

        # Reload again so widget picks up the new item
        page.reload(wait_until="domcontentloaded")
        page.wait_for_timeout(1000)
        # Use a loop with proper waiting instead of fixed timeout
        todo_text = ""
        for _ in range(10):
            todo_text = page.inner_text("#todo-container")
            if content in todo_text:
                break
            page.wait_for_timeout(500)
        assert content in todo_text, f"'{content}' not found in todo list. got: {todo_text!r}"

    def test_toggle_todo_done(self, page):
        """Toggle a todo item's done status."""
        path = tempfile.mktemp(suffix=".md")

        result = self._api_do(page, "PUT", "/api/todo/path", {"path": path})
        assert result["status"] == 200
        page.wait_for_timeout(300)
        page.reload(wait_until="domcontentloaded")
        page.wait_for_timeout(500)

        content = "[E2E] Toggle " + str(int(time.time()))
        post_result = self._api_do(page, "POST", "/api/todo", {"text": content})
        assert post_result["status"] == 200
        line_no = post_result["data"].get("line_no")
        assert line_no is not None, f"no line_no: {post_result}"

        toggle_result = self._api_do(page, "PUT", "/api/todo/" + str(line_no), {"done": True})
        assert toggle_result["status"] == 200, f"toggle failed: {toggle_result}"

    def test_delete_todo(self, page):
        """Delete a todo item and verify it's gone from UI."""
        path = tempfile.mktemp(suffix=".md")

        result = self._api_do(page, "PUT", "/api/todo/path", {"path": path})
        assert result["status"] == 200
        page.wait_for_timeout(300)
        page.reload(wait_until="domcontentloaded")
        page.wait_for_timeout(500)

        content = "[E2E] Delete " + str(int(time.time()))
        post_result = self._api_do(page, "POST", "/api/todo", {"text": content})
        assert post_result["status"] == 200
        line_no = post_result["data"].get("line_no")
        assert line_no is not None

        del_result = self._api_do(page, "DELETE", "/api/todo/" + str(line_no))
        assert del_result["status"] == 200, f"delete failed: {del_result}"

        page.reload(wait_until="domcontentloaded")
        page.wait_for_timeout(500)
        todo_text = page.inner_text("#todo-container")
        assert content not in todo_text
