from __future__ import unicode_literals
from __future__ import absolute_import
from tests import unittest

from six import StringIO

from compose import progress_stream


class ProgressStreamTestCase(unittest.TestCase):

    def test_stream_output(self):
        output = [
            '{"status": "Downloading", "progressDetail": {"current": '
            '31019763, "start": 1413653874, "total": 62763875}, '
            '"progress": "..."}',
        ]
        events = progress_stream.stream_output(output, StringIO())
        self.assertEqual(len(events), 1)

    def test_stream_output_div_zero(self):
        output = [
            '{"status": "Downloading", "progressDetail": {"current": '
            '0, "start": 1413653874, "total": 0}, '
            '"progress": "..."}',
        ]
        events = progress_stream.stream_output(output, StringIO())
        self.assertEqual(len(events), 1)

    def test_stream_output_null_total(self):
        output = [
            '{"status": "Downloading", "progressDetail": {"current": '
            '0, "start": 1413653874, "total": null}, '
            '"progress": "..."}',
        ]
        events = progress_stream.stream_output(output, StringIO())
        self.assertEqual(len(events), 1)
