"""Validate a copied package without relying on the Graybox repository layout."""
import json
from pathlib import Path
import re
import shutil
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]


class PackageTests(unittest.TestCase):
    def test_standalone_install_metadata_and_links(self):
        with tempfile.TemporaryDirectory() as directory:
            installed = Path(directory) / ".agents/skills/graybox"
            shutil.copytree(ROOT / "skills/graybox", installed)
            entry = (installed / "SKILL.md").read_text()
            self.assertTrue(entry.startswith("---\n"))
            frontmatter = entry.split("---", 2)[1]
            fields = dict(line.split(": ", 1) for line in frontmatter.strip().splitlines())
            self.assertEqual(fields["name"], installed.name)
            self.assertTrue(fields["description"].strip())
            for document in installed.rglob("*.md"):
                for link in re.findall(r"\]\(([^)]+)\)", document.read_text()):
                    if "://" not in link and not link.startswith("#"):
                        target = (document.parent / link.split("#")[0]).resolve()
                        self.assertTrue(target.is_relative_to(installed))
                        self.assertTrue(target.is_file(), str(target))

    def test_scenario_inventory_has_reproducible_inputs(self):
        scenarios = json.loads((Path(__file__).with_name("scenarios.json")).read_text())
        self.assertEqual(len({s["id"] for s in scenarios}), 7)
        for scenario in scenarios:
            for field in ("task", "baseline", "safety", "expected"):
                self.assertTrue(scenario[field].strip())


if __name__ == "__main__":
    unittest.main()
