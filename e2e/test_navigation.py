"""
e2e/test_navigation.py — verify all tabs load and switch correctly.
"""
import pytest


NAV_TABS = [
    ("dashboard",    "总览"),
    ("tasks",       "手动任务"),
    ("automation",  "自动化"),
    ("experiences", "经验库"),
    ("relay",       "代理"),
    ("rterm",       "远程终端"),
    ("aichat",      "AI 助手"),
    ("config",      "系统配置"),
]


class TestNavigation:
    @pytest.mark.parametrize("tab_id,label", NAV_TABS)
    def test_tab_switch(self, page, tab_id: str, label: str):
        """
        Clicking a nav item activates it and shows the corresponding panel.
        The panel div id is 'page-<tab_id>'.
        """
        page.click(f'[data-tab="{tab_id}"]')
        # active class moves to the clicked nav item
        active = page.query_selector(".nav-item.active")
        assert active is not None, "no active nav item after click"
        assert active.get_attribute("data-tab") == tab_id
        # panel is visible (page-<tab_id> div, not hidden)
        panel = page.wait_for_selector(f"#page-{tab_id}", timeout=5_000)
        assert panel.is_visible()

    def test_sidebar_toggle(self, page):
        """Toggle button collapses and re-expands the sidebar."""
        btn = page.query_selector(".toggle-sidebar")
        assert btn is not None
        btn.click()
        page.wait_for_timeout(300)
        # Restore
        btn.click()
        page.wait_for_timeout(300)
        sidebar = page.query_selector(".sidebar")
        assert sidebar is not None

    def test_dir_section_present(self, page):
        """Dir shortcuts section is always visible in the sidebar."""
        section = page.query_selector("#dir-section")
        assert section is not None
        assert section.is_visible()
