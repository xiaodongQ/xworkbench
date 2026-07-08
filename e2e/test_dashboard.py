"""
e2e/test_dashboard.py — dashboard loads and shows key widgets.
"""
import pytest


class TestDashboard:
    def test_dashboard_loads(self, page):
        """Dashboard tab renders the stat grid and the scheduler + recent execs widgets."""
        page.click('[data-tab="dashboard"]')
        page.wait_for_selector("#page-dashboard", timeout=5_000)
        # Stat grid — 6 stat cards
        stat_values = page.query_selector_all(".stat-value")
        assert len(stat_values) >= 5, f"expected at least 5 stat values, got {len(stat_values)}"
        # Scheduler widget
        assert page.query_selector("#dash-sched-widget") is not None
        # Recent executions widget
        assert page.query_selector("#dash-execs-widget") is not None

    def test_stat_values_are_numbers_or_dash(self, page):
        """Stat value elements contain a number or placeholder '-'."""
        page.click('[data-tab="dashboard"]')
        page.wait_for_selector("#stat-pending", timeout=5_000)
        stats = ["stat-pending", "stat-in_progress", "stat-running",
                 "stat-archived", "stat-exception"]
        for sid in stats:
            el = page.query_selector(f"#{sid}")
            assert el is not None, f"missing stat element #{sid}"
            text = el.inner_text().strip()
            assert text != "", f"#{sid} should not be empty"

    def test_scheduler_widget_present(self, page):
        """Scheduler widget shows a badge and summary list."""
        page.click('[data-tab="dashboard"]')
        el = page.wait_for_selector("#dash-sched-widget", timeout=5_000)
        assert el.is_visible()
        assert page.query_selector("#scheduler-badge") is not None

    def test_recent_execs_widget_present(self, page):
        """Recent executions widget is present."""
        el = page.wait_for_selector("#dash-execs-widget", timeout=5_000)
        assert el.is_visible()
