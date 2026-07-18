import json
import unittest
from pathlib import Path
from runner import _sha256, load_manifest, run_case, verify_hashes
from incremental import measure_stale_rate

class RunnerTests(unittest.TestCase):
    def setUp(self):
        self.root = Path(__file__).resolve().parents[3] / 'benchmarks/victory-lab/v1'
        self.manifest = load_manifest(self.root / 'manifest.json')

    def test_manifest_cases_execute(self):
        self.assertEqual(len(self.manifest['cases']), 9)
        for case in self.manifest['cases']:
            self.assertEqual(run_case(self.root, case, self.manifest)['status'], 'PASS', case['id'])

    def test_fixture_metrics_are_not_all_perfect(self):
        results = [run_case(self.root, case, self.manifest) for case in self.manifest['cases']]
        self.assertTrue(any(r['quality_status'] == 'MEASURED_NON_PERFECT' for r in results))
        relations = json.loads((self.root / 'goldens/relations.json').read_text())['relations']
        self.assertEqual({r['kind'] for r in relations}, {'positive', 'negative', 'ambiguous', 'unresolved', 'not-comparable'})

    def test_stale_rate_is_measured_from_changed_state(self):
        case = self.manifest['cases'][0]
        paths = [p for pattern in case['corpus'] for p in (self.root / pattern).rglob('*') if p.is_file()]
        result = measure_stale_rate(self.root, paths, self.manifest.get('extensions', {}))
        self.assertTrue(result['states_differ'])
        self.assertEqual(result['operations'], ['create', 'change', 'delete', 'rename'])
        self.assertEqual(result['stale_rate'], result['stale_record_count'] / result['comparable_records'])
        self.assertTrue(result['clean_equivalent'])

    def test_manifest_hashes_are_line_ending_stable_and_mutation_sensitive(self):
        import tempfile
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            fixture = root / 'fixture.txt'
            fixture.write_bytes(b'alpha\nbeta\n')
            manifest = {'hashes': {'fixture.txt': _sha256(fixture)}}
            fixture.write_bytes(b'alpha\r\nbeta\r\n')
            verify_hashes(root, manifest)
            fixture.write_bytes(b'alpha\r\nmutated\r\n')
            with self.assertRaisesRegex(ValueError, 'hash mismatch'):
                verify_hashes(root, manifest)
