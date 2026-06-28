from pathlib import Path
from types import SimpleNamespace
import tempfile
import unittest

from jinja2 import Environment, FileSystemLoader, select_autoescape


ROOT = Path(__file__).resolve().parents[1]
THEME_DIR = ROOT / "cloudboss_docs" / "theme"


class ThemeAnalyticsTests(unittest.TestCase):
    def render_theme(self, extra=None):
        with tempfile.TemporaryDirectory() as tmp:
            Path(tmp, "base.html").write_text(
                "<html><head>"
                "{% block analytics %}MATERIAL-ANALYTICS{% endblock %}"
                "{% block extrahead %}{% endblock %}"
                "</head></html>",
                encoding="utf-8",
            )
            env = Environment(
                loader=FileSystemLoader([tmp, str(THEME_DIR)]),
                autoescape=select_autoescape(["html", "xml"]),
            )
            template = env.get_template("main.html")
            return template.render(config=SimpleNamespace(extra=extra or {}))

    def test_omits_google_analytics_without_property(self):
        html = self.render_theme()

        self.assertNotIn("googletagmanager.com", html)
        self.assertNotIn("gtag(", html)
        self.assertNotIn("MATERIAL-ANALYTICS", html)

    def test_includes_google_analytics_when_property_is_set(self):
        html = self.render_theme({"google_analytics_property": "G-TEST123"})

        self.assertIn(
            "https://www.googletagmanager.com/gtag/js?id=G-TEST123",
            html,
        )
        self.assertIn('gtag("config", "G-TEST123");', html)
        self.assertNotIn("MATERIAL-ANALYTICS", html)

    def test_ignores_old_cloudboss_google_analytics_config(self):
        html = self.render_theme(
            {"cloudboss_google_analytics": {"property": "G-OLD123"}}
        )

        self.assertNotIn("googletagmanager.com", html)
        self.assertNotIn("G-OLD123", html)
        self.assertNotIn("MATERIAL-ANALYTICS", html)

    def test_material_analytics_config_is_disabled(self):
        html = self.render_theme(
            {"analytics": {"provider": "google", "property": "G-MATERIAL"}}
        )

        self.assertNotIn("googletagmanager.com", html)
        self.assertNotIn("G-MATERIAL", html)
        self.assertNotIn("MATERIAL-ANALYTICS", html)


if __name__ == "__main__":
    unittest.main()
