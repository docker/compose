from __future__ import absolute_import
from __future__ import unicode_literals

from six import StringIO

from compose import progress_stream
from tests import unittest


class ProgressStreamTestCase(unittest.TestCase):
    def test_stream_output(self):
        output = [
            b'{"status": "Downloading", "progressDetail": {"current": '
            b'31019763, "start": 1413653874, "total": 62763875}, '
            b'"progress": "..."}',
        ]
        events = progress_stream.stream_output(output, StringIO())
        self.assertEqual(len(events), 1)

    def test_stream_output_div_zero(self):
        output = [
            b'{"status": "Downloading", "progressDetail": {"current": '
            b'0, "start": 1413653874, "total": 0}, '
            b'"progress": "..."}',
        ]
        events = progress_stream.stream_output(output, StringIO())
        self.assertEqual(len(events), 1)

    def test_stream_output_null_total(self):
        output = [
            b'{"status": "Downloading", "progressDetail": {"current": '
            b'0, "start": 1413653874, "total": null}, '
            b'"progress": "..."}',
        ]
        events = progress_stream.stream_output(output, StringIO())
        self.assertEqual(len(events), 1)
