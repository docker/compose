from __future__ import absolute_import
from __future__ import unicode_literals

import unittest
from time import sleep

from compose.cli.multiplexer import Multiplexer


class MultiplexerTest(unittest.TestCase):
    def test_no_iterators(self):
        mux = Multiplexer([])
        self.assertEqual([], list(mux.loop()))

    def test_empty_iterators(self):
        mux = Multiplexer([
            (x for x in []),
            (x for x in []),
        ])

        self.assertEqual([], list(mux.loop()))

    def test_aggregates_output(self):
        mux = Multiplexer([
            (x for x in [0, 2, 4]),
            (x for x in [1, 3, 5]),
        ])

        self.assertEqual(
            [0, 1, 2, 3, 4, 5],
            sorted(list(mux.loop())),
        )

    def test_exception(self):
        class Problem(Exception):
            pass

        def problematic_iterator():
            yield 0
            yield 2
            raise Problem(":(")

        mux = Multiplexer([
            problematic_iterator(),
            (x for x in [1, 3, 5]),
        ])

        with self.assertRaises(Problem):
            list(mux.loop())

    def test_cascade_stop(self):
        mux = Multiplexer([
            ((lambda x: sleep(0.01) or x)(x) for x in ['after 0.01 sec T1',
                                                       'after 0.02 sec T1',
                                                       'after 0.03 sec T1']),
            ((lambda x: sleep(0.02) or x)(x) for x in ['after 0.02 sec T2',
                                                       'after 0.04 sec T2',
                                                       'after 0.06 sec T2']),
        ], cascade_stop=True)

        self.assertEqual(
            ['after 0.01 sec T1',
             'after 0.02 sec T1',
             'after 0.02 sec T2',
             'after 0.03 sec T1'],
            sorted(list(mux.loop())))
