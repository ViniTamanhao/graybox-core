"""Evidence checks, not a simulation of agent behavior."""
import unittest
from evaluate import verdict


class VerdictTests(unittest.TestCase):
    def report(self, outcomes):
        return {"summary": {"total": len(outcomes), **{
            name: outcomes.count(name) for name in ("equivalent", "changed", "failed")}},
            "results": [{"exchange_id": i + 1, "outcome": value}
                        for i, value in enumerate(outcomes)]}

    def test_nonempty_equivalence(self):
        self.assertEqual(verdict(0, self.report(["equivalent"])), "equivalent")

    def test_empty_is_unverified(self):
        self.assertEqual(verdict(0, self.report([])), "unverified")

    def test_failure_dominates_change(self):
        self.assertEqual(verdict(3, self.report(["changed", "failed"])), "failed")

    def test_changed_is_not_regression_classification(self):
        self.assertEqual(verdict(1, self.report(["changed"])), "changed")

    def test_inconsistent_exit_counts_and_outcomes(self):
        report = self.report(["equivalent"])
        self.assertEqual(verdict(1, report), "unverified")
        report["summary"]["total"] = 2
        self.assertEqual(verdict(0, report), "unverified")
        self.assertEqual(verdict(0, self.report(["unknown"])), "unverified")

    def test_command_errors_or_missing_report(self):
        for code in (2, 4, 5):
            self.assertEqual(verdict(code, self.report(["equivalent"])), "unverified")
        self.assertEqual(verdict(0, None), "unverified")


if __name__ == "__main__":
    unittest.main()
