import unittest
from metrics import classification_metrics, latency_summary, mad, percentile, bootstrap_mean_ci, relation_metrics

class MetricsTests(unittest.TestCase):
    def test_classification(self):
        self.assertEqual(classification_metrics(8, 2, 0)['precision'], .8)
        self.assertEqual(classification_metrics(8, 2, 0)['recall'], 1.0)

    def test_robust_latency(self):
        self.assertEqual(percentile([1,2,3,4], 50), 2.5)
        self.assertEqual(mad([1,2,3]), 1)
        self.assertEqual(latency_summary([1,2])['n'], 2)

    def test_seed(self):
        self.assertEqual(bootstrap_mean_ci([1,2,3], seed=7), bootstrap_mean_ci([1,2,3], seed=7))

    def test_relation_classes_keep_unsupported_out_of_quality(self):
        metrics = relation_metrics([{'from':'a#Consumer','to':'b#Known'}], [{'from':'Consumer','to':'Known','kind':'positive'}, {'from':'Consumer','to':'Missing','kind':'unresolved'}])
        self.assertEqual(metrics['tp'], 1)
        self.assertEqual(metrics['unresolved'], 1)
