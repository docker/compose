from __future__ import unicode_literals
from __future__ import absolute_import
from fig.cli.utils import split_buffer
from . import unittest

class SplitBufferTest(unittest.TestCase):
    def test_single_line_chunks(self):
        def reader():
            yield "abc\n"
            yield "def\n"
            yield "ghi\n"

        self.assertEqual(list(split_buffer(reader(), '\n')), ["abc\n", "def\n", "ghi\n"])

    def test_no_end_separator(self):
        def reader():
            yield "abc\n"
            yield "def\n"
            yield "ghi"

        self.assertEqual(list(split_buffer(reader(), '\n')), ["abc\n", "def\n", "ghi"])

    def test_multiple_line_chunk(self):
        def reader():
            yield "abc\ndef\nghi"

        self.assertEqual(list(split_buffer(reader(), '\n')), ["abc\n", "def\n", "ghi"])

    def test_chunked_line(self):
        def reader():
            yield "a"
            yield "b"
            yield "c"
            yield "\n"
            yield "d"

        self.assertEqual(list(split_buffer(reader(), '\n')), ["abc\n", "d"])
