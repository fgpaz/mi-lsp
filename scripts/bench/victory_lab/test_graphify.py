import unittest
from pathlib import Path
from graphify import extract, GRAPHIFY_REVISION

class GraphifyTests(unittest.TestCase):
    def test_revision_and_symbols(self):
        f = Path(__file__).resolve().parents[3] / 'benchmarks/victory-lab/v1/corpus/csharp/Greeter.cs'
        g = extract([f])
        self.assertEqual(g['graphify_revision'], GRAPHIFY_REVISION)
        self.assertEqual({x['name'] for x in g['symbols']}, {'Greeter', 'Greet'})

    def test_extension_parser(self):
        f = Path(__file__).resolve().parents[3] / 'benchmarks/victory-lab/v1/corpus/extensions/custom.foo'
        self.assertEqual({x['name'] for x in extract([f], {'.foo':'extension'})['symbols']}, {'ExtensionNode', 'ExtensionEdge'})

    def test_ambiguous_relationship_is_not_claimed_as_edge(self):
        root = Path(__file__).resolve().parents[3] / 'benchmarks/victory-lab/v1/corpus/relations'
        graph = extract(list(root.glob('*.py')))
        self.assertTrue(graph['diagnostics']['ambiguous'])
        self.assertFalse(any(edge['to'].endswith('#Shared') for edge in graph['edges']))
