import importlib.util
from pathlib import Path
import unittest
import subprocess
import sys
import tempfile

spec = importlib.util.spec_from_file_location("release_notes", Path(__file__).with_name("release-notes.py"))
notes = importlib.util.module_from_spec(spec)
spec.loader.exec_module(notes)


class ReleaseNotesTest(unittest.TestCase):
    def test_extracts_only_requested_version_and_preserves_markdown(self):
        text = """# Changelog

## [Unreleased]

- Later change.

## [0.4.0] - 2026-11-10

### Added

- **Review cleanup.** Inspect files before removal.

### Fixed

- Keep protected files.

## [0.3.0] - 2026-10-29

- Earlier change.
"""
        self.assertEqual(notes.extract_notes(text, "v0.4.0"),
                         "### Added\n\n- **Review cleanup.** Inspect files before removal.\n\n"
                         "### Fixed\n\n- Keep protected files.\n")

    def test_rejects_missing_duplicate_empty_or_invalid_release(self):
        valid = "## [0.4.0] - 2026-11-10\n\n- Change.\n"
        for text, tag in [
            (valid, "v0.5.0"),
            (valid + valid, "v0.4.0"),
            ("## [0.4.0] - 2026-11-10\n\n### Added\n", "v0.4.0"),
            (valid.replace("2026-11-10", "2026-02-30"), "v0.4.0"),
            (valid.replace("2026-11-10", "2026-11-10 [YANKED]"), "v0.4.0"),
            (valid, "0.4.0"),
            (valid, "v0.4.0-rc1"),
        ]:
            with self.subTest(text=text, tag=tag):
                with self.assertRaises(ValueError):
                    notes.extract_notes(text, tag)

    def test_command_prints_notes_and_rejects_missing_file_without_output(self):
        script = Path(__file__).with_name("release-notes.py")
        with tempfile.TemporaryDirectory() as tmp:
            changelog = Path(tmp) / "CHANGELOG.md"
            changelog.write_text("## [0.4.0] - 2026-11-10\n\n- Reviewed changes.\n")
            command = [sys.executable, "-B", str(script), "v0.4.0", str(changelog)]
            result = subprocess.run(command, capture_output=True, text=True)
            self.assertEqual(result.returncode, 0)
            self.assertEqual(result.stdout, "- Reviewed changes.\n")
            changelog.unlink()
            result = subprocess.run(command, capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertEqual(result.stdout, "")
            self.assertIn("error:", result.stderr)


if __name__ == "__main__":
    unittest.main()
