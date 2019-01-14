# ~*~ encoding: utf-8 ~*~
from __future__ import absolute_import
from __future__ import unicode_literals

import io
import os
import random
import shutil
import tempfile

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
        events = list(progress_stream.stream_output(output, StringIO()))
        assert len(events) == 1

    def test_stream_output_div_zero(self):
        output = [
            b'{"status": "Downloading", "progressDetail": {"current": '
            b'0, "start": 1413653874, "total": 0}, '
            b'"progress": "..."}',
        ]
        events = list(progress_stream.stream_output(output, StringIO()))
        assert len(events) == 1

    def test_stream_output_null_total(self):
        output = [
            b'{"status": "Downloading", "progressDetail": {"current": '
            b'0, "start": 1413653874, "total": null}, '
            b'"progress": "..."}',
        ]
        events = list(progress_stream.stream_output(output, StringIO()))
        assert len(events) == 1

    def test_stream_output_progress_event_tty(self):
        events = [
            b'{"status": "Already exists", "progressDetail": {}, "id": "8d05e3af52b0"}'
        ]

        class TTYStringIO(StringIO):
            def isatty(self):
                return True

        output = TTYStringIO()
        events = list(progress_stream.stream_output(events, output))
        assert len(output.getvalue()) > 0

    def test_stream_output_progress_event_no_tty(self):
        events = [
            b'{"status": "Already exists", "progressDetail": {}, "id": "8d05e3af52b0"}'
        ]
        output = StringIO()

        events = list(progress_stream.stream_output(events, output))
        assert len(output.getvalue()) == 0

    def test_stream_output_no_progress_event_no_tty(self):
        events = [
            b'{"status": "Pulling from library/xy", "id": "latest"}'
        ]
        output = StringIO()

        events = list(progress_stream.stream_output(events, output))
        assert len(output.getvalue()) > 0

    def test_mismatched_encoding_stream_write(self):
        tmpdir = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, tmpdir, True)

        def mktempfile(encoding):
            fname = os.path.join(tmpdir, hex(random.getrandbits(128))[2:-1])
            return io.open(fname, mode='w+', encoding=encoding)

        text = '就吃饭'
        with mktempfile(encoding='utf-8') as tf:
            progress_stream.write_to_stream(text, tf)
            tf.seek(0)
            assert tf.read() == text

        with mktempfile(encoding='utf-32') as tf:
            progress_stream.write_to_stream(text, tf)
            tf.seek(0)
            assert tf.read() == text

        with mktempfile(encoding='ascii') as tf:
            progress_stream.write_to_stream(text, tf)
            tf.seek(0)
            assert tf.read() == '???'

    def test_get_digest_from_push(self):
        digest = "sha256:abcd"
        events = [
            {"status": "..."},
            {"status": "..."},
            {"progressDetail": {}, "aux": {"Digest": digest}},
        ]
        assert progress_stream.get_digest_from_push(events) == digest

    def test_get_digest_from_pull(self):
        events = list()
        assert progress_stream.get_digest_from_pull(events) is None

        digest = "sha256:abcd"
        events = [
            {"status": "..."},
            {"status": "..."},
            {"status": "Digest: %s" % digest},
            {"status": "..."},
        ]
        assert progress_stream.get_digest_from_pull(events) == digest
