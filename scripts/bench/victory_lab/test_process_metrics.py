import tempfile,unittest
from pathlib import Path
from process_metrics import directory_bytes,snapshot
class ProcessMetricTests(unittest.TestCase):
 def test_snapshot_labels(self):
  s=snapshot(); self.assertIn('os',s); self.assertIn('arch',s); self.assertIn('unit_version',s)
 def test_directory_bytes(self):
  with tempfile.TemporaryDirectory() as d: (Path(d)/'x').write_bytes(b'123'); self.assertEqual(directory_bytes(d),3)
