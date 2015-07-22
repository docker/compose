import unittest

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
