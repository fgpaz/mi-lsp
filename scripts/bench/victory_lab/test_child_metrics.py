import unittest

try:
    from .child_metrics import ChildMetrics, _Sampler
except ImportError:
    from child_metrics import ChildMetrics, _Sampler


class _Probe:
    def __init__(self, values, descendants=()):
        self.values = dict(values)
        self._descendants = set(descendants)

    def working_set(self, pid):
        return self.values.get(pid)

    def descendants(self, pid):
        return set(self._descendants)


class ChildMetricsFocusedTests(unittest.TestCase):
    def test_singleton_root_sample_is_complete_tree_coverage(self):
        sampler = _Sampler(101, _Probe({101: 100}), interval=1.0)
        sampler._sample()

        metrics = sampler.metrics(
            pid=101, returncode=0, timed_out=False, crashed=False,
            cleanup_status="clean",
        )

        self.assertEqual(metrics.status, "PASS")
        self.assertTrue(metrics.tree_supported)
        self.assertEqual(metrics.peak_rss_bytes, 100)
        self.assertEqual(metrics.tree_peak_rss_bytes, 100)
        self.assertEqual(metrics.observed_pids, (101,))
        self.assertNotIn("tree_not_observed", metrics.reason_codes)

    def test_inconsistent_pass_is_normalized_without_descendants(self):
        metrics = ChildMetrics(
            peak_rss_bytes=100, status="PASS", tree_peak_rss_bytes=100,
            tree_supported=False, observed_pids=(101,),
        )

        self.assertEqual(metrics.status, "NOT_COMPARABLE")
        self.assertEqual(metrics.reason, "tree_not_observed")
        self.assertIsNone(metrics.tree_peak_rss_bytes)
        self.assertEqual(metrics.observed_pids, (101,))

    def test_partial_child_working_set_is_not_comparable_and_has_no_tree_peak(self):
        sampler = _Sampler(101, _Probe({101: 100, 202: None}, descendants={202}), interval=1.0)
        sampler._sample()

        metrics = sampler.metrics(
            pid=101, returncode=0, timed_out=False, crashed=False,
            cleanup_status="clean",
        )

        self.assertEqual(metrics.status, "NOT_COMPARABLE")
        self.assertFalse(metrics.tree_supported)
        self.assertIsNone(metrics.tree_peak_rss_bytes)
        self.assertEqual(metrics.peak_rss_bytes, 100)
        self.assertEqual(metrics.observed_pids, (101, 202))
        self.assertEqual(metrics.reason, "working_set_unavailable")
        self.assertIn("working_set_unavailable", metrics.reason_codes)

    def test_complete_root_and_child_observation_keeps_tree_total(self):
        sampler = _Sampler(101, _Probe({101: 100, 202: 200}, descendants={202}), interval=1.0)
        sampler._sample()

        metrics = sampler.metrics(
            pid=101, returncode=0, timed_out=False, crashed=False,
            cleanup_status="clean",
        )

        self.assertEqual(metrics.status, "PASS")
        self.assertTrue(metrics.tree_supported)
        self.assertEqual(metrics.tree_peak_rss_bytes, 300)
        self.assertEqual(metrics.observed_pids, (101, 202))

    def test_partial_descendant_stays_not_comparable_after_later_sample(self):
        class _SequenceProbe(_Probe):
            def __init__(self):
                super().__init__({101: 100, 202: 200})
                self.calls = 0

            def descendants(self, pid):
                self.calls += 1
                return {202}

            def working_set(self, pid):
                if pid == 202 and self.calls > 1:
                    return None
                return super().working_set(pid)

        sampler = _Sampler(101, _SequenceProbe(), interval=1.0)
        sampler._sample()
        sampler._sample()

        metrics = sampler.metrics(
            pid=101, returncode=0, timed_out=False, crashed=False,
            cleanup_status="clean",
        )

        self.assertEqual(metrics.status, "NOT_COMPARABLE")
        self.assertFalse(metrics.tree_supported)
        self.assertIsNone(metrics.tree_peak_rss_bytes)
        self.assertEqual(metrics.reason, "working_set_unavailable")
        self.assertIn("working_set_unavailable", metrics.reason_codes)


if __name__ == "__main__":
    unittest.main()
