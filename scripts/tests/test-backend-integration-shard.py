"""The sharded backend integration gate runs every test exactly once.

Static and pure checks only (standard library): the partition, the run pattern,
the package exclusion of the rest leg, and the matrix in backend.yml.
"""
from pathlib import Path
import importlib.util
import random
import re
import unittest

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('shard', ROOT / 'scripts/backend-integration-shard.py')
shard = importlib.util.module_from_spec(spec)
spec.loader.exec_module(shard)


class PartitionTests(unittest.TestCase):
    def names(self):
        rng = random.Random(20261116)
        return sorted({'Test' + ''.join(rng.choice('ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_') for _ in range(rng.randint(3, 60))) for _ in range(800)})

    def test_every_test_is_selected_by_exactly_one_shard(self):
        names = self.names()
        for shards in (1, 2, 3, 4, 6):
            chosen = [shard.select(names, i, shards) for i in range(shards)]
            flat = [n for part in chosen for n in part]
            self.assertEqual(sorted(flat), names)
            self.assertEqual(len(flat), len(set(flat)))
            if shards > 1:
                self.assertTrue(all(chosen), 'an empty shard for a large package')

    def test_partition_is_stable_and_ignores_order(self):
        names = self.names()
        shuffled = list(reversed(names))
        for i in range(4):
            self.assertEqual(sorted(shard.select(names, i, 4)), sorted(shard.select(shuffled, i, 4)))
        self.assertEqual(shard.bucket('TestTripStopsOwnReadKeepsThePlanApartFromObservedTimes', 4),
                         shard.bucket('TestTripStopsOwnReadKeepsThePlanApartFromObservedTimes', 4))

    def test_out_of_range_shard_is_refused(self):
        for i, n in ((4, 4), (-1, 2), (0, 0)):
            with self.assertRaises(ValueError):
                shard.select(['TestA'], i, n)

    def test_run_pattern_is_anchored_so_prefixes_never_widen_a_shard(self):
        pattern = re.compile(shard.run_pattern(['TestA', 'ExampleB', 'FuzzC']))
        self.assertTrue(pattern.match('TestA'))
        self.assertTrue(pattern.match('FuzzC'))
        self.assertFalse(pattern.match('TestAB'))
        self.assertFalse(pattern.match('XTestA'))

    def test_listing_keeps_tests_examples_and_fuzz_seeds_only(self):
        for good in ('TestX', 'Example_y', 'FuzzZ'):
            self.assertTrue(shard.NAME.match(good))
        for bad in ('ok  \tpkg', 'BenchmarkX', '', 'PASS'):
            self.assertFalse(shard.NAME.match(bad))

    def test_rest_leg_excludes_exactly_the_sharded_packages(self):
        listed = ['m/a', 'm/internal/modules/organization/adapters', 'm/b', 'm/internal/platform/apicontract']
        rest = shard.rest_packages(listed, {'m/internal/modules/organization/adapters', 'm/internal/platform/apicontract'})
        self.assertEqual(rest, ['m/a', 'm/b'])
        self.assertEqual(shard.GO_TEST, ['go', 'test', '-race', '-count=1', '-timeout=45m', '-tags=integration'])


class WorkflowTests(unittest.TestCase):
    def setUp(self):
        raw = (ROOT / '.github/workflows/backend.yml').read_text()
        self.integration = raw.split('\n  integration:\n', 1)[1]

    def legs(self):
        return re.findall(r'(?m)^          - \{ leg: "([^"]+)", package: "([^"]*)", shard: (\d+), shards: (\d+) \}$', self.integration)

    def test_matrix_covers_every_configured_shard_once_and_one_rest_leg(self):
        legs = self.legs()
        self.assertTrue(legs)
        seen = {}
        for _, package, index, total in legs:
            seen.setdefault((package, int(total)), []).append(int(index))
        expected = {(p, n): list(range(n)) for p, n in shard.SHARDED.items()}
        expected[('', 1)] = [0]
        self.assertEqual({k: sorted(v) for k, v in seen.items()}, expected)
        self.assertIn('fail-fast: false', self.integration)

    def test_every_leg_runs_the_shard_script_with_real_services_and_no_direct_package_run(self):
        self.assertIn('scripts/backend-integration-shard.py', self.integration)
        commands = '\n'.join(line for line in self.integration.splitlines() if not line.lstrip().startswith('#'))
        self.assertNotRegex(commands, r'go test [^\n]*\./\.\.\.')
        for service in ('postgres:', 'storage:', 'go run ./cmd/migrate'):
            self.assertIn(service, self.integration)

    def test_rust_preparers_are_built_for_the_rest_leg(self):
        rust = self.integration.split('- name: Build locked Rust source preparers', 1)[1].split('\n      - name:', 1)[0]
        self.assertIn("if: matrix.package == ''", rust)


if __name__ == '__main__':
    unittest.main()
