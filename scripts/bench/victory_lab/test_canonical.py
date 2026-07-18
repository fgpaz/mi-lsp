import unittest
from canonical import canonical_json,comparable_units,token_count
class CanonicalTests(unittest.TestCase):
 def test_stable_order_and_volatile_removal(self): self.assertEqual(canonical_json({"b":1,"a":2,"pid":4}),'{"a":2,"b":1}')
 def test_units(self): self.assertEqual(comparable_units('{"a":1}'),comparable_units('{"a":1}')); self.assertGreater(token_count('{"a":1}'),0)
