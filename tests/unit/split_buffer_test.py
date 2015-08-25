from __future__ import absolute_import
from __future__ import unicode_literals

from .. import unittest
from compose.cli.utils import split_buffer


class SplitBufferTest(unittest.TestCase):
    def test_single_line_chunks(self):
        def reader():
            yield 'abc\n'
            yield 'def\n'
            yield 'ghi\n'

        self.assert_produces(reader, ['abc\n', 'def\n', 'ghi\n'])

    def test_no_end_separator(self):
        def reader():
            yield 'abc\n'
            yield 'def\n'
            yield 'ghi'

        self.assert_produces(reader, ['abc\n', 'def\n', 'ghi'])

    def test_multiple_line_chunk(self):
        def reader():
            yield 'abc\ndef\nghi'

        self.assert_produces(reader, ['abc\n', 'def\n', 'ghi'])

    def test_chunked_line(self):
        def reader():
            yield 'a'
            yield 'b'
            yield 'c'
            yield '\n'
            yield 'd'

        self.assert_produces(reader, ['abc\n', 'd'])

    def test_preserves_unicode_sequences_within_lines(self):
        string = u"a\u2022c\n"

        def reader():
            yield string

        self.assert_produces(reader, [string])

    def assert_produces(self, reader, expectations):
        split = split_buffer(reader(), '\n')

        for (actual, expected) in zip(split, expectations):
            self.assertEqual(type(actual), type(expected))
            self.assertEqual(actual, expected)
