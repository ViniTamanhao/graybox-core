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

    def test_portable_installations_refuse_overwrite(self):
        # Exercise the guide's POSIX copy command outside this repository.
        import subprocess
        for location in (".agents/skills", ".claude/skills", ".cursor/skills", ".github/skills"):
            with self.subTest(location=location), tempfile.TemporaryDirectory() as directory:
                destination = Path(directory) / location / "graybox"
                command = 'mkdir -p "$skill_parent" && test ! -e "$skill_parent/graybox" && test ! -L "$skill_parent/graybox" && cp -R "$graybox_source/skills/graybox" "$skill_parent/graybox"'
                import os
                env = dict(os.environ, skill_parent=str(destination.parent), graybox_source=str(ROOT))
                self.assertEqual(subprocess.run(["sh", "-c", command], env=env).returncode, 0)
                before = {str(p.relative_to(destination)): p.read_bytes() for p in destination.rglob("*") if p.is_file()}
                self.assertEqual(len(before), 5)
                self.assertNotEqual(subprocess.run(["sh", "-c", command], env=env).returncode, 0)
                self.assertEqual(before, {str(p.relative_to(destination)): p.read_bytes() for p in destination.rglob("*") if p.is_file()})
                tracked = subprocess.check_output(["git", "ls-files", "skills/graybox"], cwd=ROOT, text=True).splitlines()
                self.assertEqual(set(before), {str(Path(p).relative_to("skills/graybox")) for p in tracked})

    def test_minimal_standard_frontmatter(self):
        entry = (ROOT / "skills/graybox/SKILL.md").read_text()
        header = entry.split("---", 2)[1].strip().splitlines()
        fields = dict(line.split(": ", 1) for line in header)
        self.assertEqual(set(fields), {"name", "description"})
        self.assertEqual(fields["name"], "graybox")
        self.assertLessEqual(len(fields["description"]), 1024)
        self.assertTrue(all(word in fields["description"] for word in ("HTTP", "authentication", "regressions", "documentation-only")))

    def test_scenario_inventory_has_reproducible_inputs(self):
        scenarios = json.loads((Path(__file__).with_name("scenarios.json")).read_text())
        self.assertEqual(len({s["id"] for s in scenarios}), 7)
        for scenario in scenarios:
            for field in ("task", "baseline", "safety", "expected"):
                self.assertTrue(scenario[field].strip())


if __name__ == "__main__":
    unittest.main()
