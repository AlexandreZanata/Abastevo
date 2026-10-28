import importlib.util
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('regions', Path(__file__).with_name('generate-map-regions.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


def poly(name, rings):
    lines = [name]
    for index, (hole, points) in enumerate(rings, start=1):
        lines.append(('!' if hole else '') + str(index))
        lines += [f'\t{x:.6E}\t{y:.6E}' for x, y in points]
        lines.append('END')
    lines.append('END')
    return '\n'.join(lines) + '\n'


class GenerateMapRegionsTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        # A dense square (collinear points collapse) with a hole, plus a second leaf with a space.
        dense = [(i / 100, 0.0) for i in range(100)] + [(1.0, i / 100) for i in range(100)] + \
            [(1.0 - i / 100, 1.0) for i in range(100)] + [(0.0, 1.0 - i / 100) for i in range(100)]
        hole = [(0.25, 0.25), (0.75, 0.25), (0.75, 0.75), (0.25, 0.75)]
        (self.root / 'Brazil_A.poly').write_text(poly('Brazil_A', [(False, dense), (True, hole)]))
        (self.root / 'Brazil_B C.poly').write_text(poly('Brazil_B C', [(False, [(2, 0), (3, 0), (3, 1), (2, 1), (2, 0)])]))
        (self.root / 'Argentina_X.poly').write_text(poly('Argentina_X', [(False, [(5, 5), (6, 5), (6, 6)])]))

    def generate(self, out):
        module.main(['--borders', str(self.root), '--out', str(out), '--expected-count', '2'])
        return out.read_text().splitlines()

    def test_simplifies_keeps_holes_and_is_reproducible(self):
        first = self.generate(self.root / 'a.txt')
        second = self.generate(self.root / 'b.txt')
        self.assertEqual(first, second)
        self.assertEqual(first[0], module.FORMAT)
        body = [line for line in first if not line.startswith('#')][1:]
        self.assertEqual(body[0], 'R\tBrazil_A')
        self.assertTrue(body[1].startswith('O\t0,0;'))
        self.assertEqual(len(body[1].split(';')), 4, 'collinear vertices collapse to the 4 corners')
        self.assertTrue(body[2].startswith('H\t2500,2500;'))
        self.assertEqual(body[3], 'R\tBrazil_B C')
        self.assertEqual(body[4], 'O\t20000,0;10000,0;0,10000;-10000,0')

    def test_refuses_unexpected_region_count(self):
        with self.assertRaises(SystemExit):
            module.main(['--borders', str(self.root), '--out', str(self.root / 'x.txt'), '--expected-count', '23'])

    def test_simplify_ring_never_degenerates(self):
        ring = module.simplify_ring([(0, 0), (1e-9, 0), (0, 1e-9)], 0.01)
        self.assertEqual(len(ring), 3)


if __name__ == '__main__':
    unittest.main()
