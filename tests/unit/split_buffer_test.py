from __future__ import unicode_literals
from __future__ import absolute_import
from compose.cli.utils import split_buffer
from .. import unittest


class SplitBufferTest(unittest.TestCase):
    def test_single_line_chunks(self):
        def reader():
            yield b'abc\n'
            yield b'def\n'
            yield b'ghi\n'

        self.assert_produces(reader, [b'abc\n', b'def\n', b'ghi\n'])

    def test_no_end_separator(self):
        def reader():
            yield b'abc\n'
            yield b'def\n'
            yield b'ghi'

        self.assert_produces(reader, [b'abc\n', b'def\n', b'ghi'])

    def test_multiple_line_chunk(self):
        def reader():
            yield b'abc\ndef\nghi'

        self.assert_produces(reader, [b'abc\n', b'def\n', b'ghi'])

    def test_chunked_line(self):
        def reader():
            yield b'a'
            yield b'b'
            yield b'c'
            yield b'\n'
            yield b'd'

        self.assert_produces(reader, [b'abc\n', b'd'])

    def test_preserves_unicode_sequences_within_lines(self):
        string = u"a\u2022c\n".encode('utf-8')

        def reader():
            yield string

        self.assert_produces(reader, [string])

    def assert_produces(self, reader, expectations):
        split = split_buffer(reader(), b'\n')

        for (actual, expected) in zip(split, expectations):
            self.assertEqual(type(actual), type(expected))
            self.assertEqual(actual, expected)
