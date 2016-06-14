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

    def test_stream_output_progress_event_tty(self):
        events = [
            b'{"status": "Already exists", "progressDetail": {}, "id": "8d05e3af52b0"}'
        ]

        class TTYStringIO(StringIO):
            def isatty(self):
                return True

        output = TTYStringIO()
        events = progress_stream.stream_output(events, output)
        self.assertTrue(len(output.getvalue()) > 0)

    def test_stream_output_progress_event_no_tty(self):
        events = [
            b'{"status": "Already exists", "progressDetail": {}, "id": "8d05e3af52b0"}'
        ]
        output = StringIO()

        events = progress_stream.stream_output(events, output)
        self.assertEqual(len(output.getvalue()), 0)

    def test_stream_output_no_progress_event_no_tty(self):
        events = [
            b'{"status": "Pulling from library/xy", "id": "latest"}'
        ]
        output = StringIO()

        events = progress_stream.stream_output(events, output)
        self.assertTrue(len(output.getvalue()) > 0)


def test_get_digest_from_push():
    digest = "sha256:abcd"
    events = [
        {"status": "..."},
        {"status": "..."},
        {"progressDetail": {}, "aux": {"Digest": digest}},
    ]
    assert progress_stream.get_digest_from_push(events) == digest


def test_get_digest_from_pull():
    digest = "sha256:abcd"
    events = [
        {"status": "..."},
        {"status": "..."},
        {"status": "Digest: %s" % digest},
    ]
    assert progress_stream.get_digest_from_pull(events) == digest
