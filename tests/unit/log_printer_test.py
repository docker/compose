from __future__ import unicode_literals
from __future__ import absolute_import
import os

from fig.cli.log_printer import LogPrinter
from .. import unittest


class LogPrinterTest(unittest.TestCase):
    def test_single_container(self):
        def reader(*args, **kwargs):
            yield "hello\nworld"

        container = MockContainer(reader)
        output = run_log_printer([container])

        self.assertIn('hello', output)
        self.assertIn('world', output)

    def test_unicode(self):
        glyph = u'\u2022'.encode('utf-8')

        def reader(*args, **kwargs):
            yield glyph + b'\n'

        container = MockContainer(reader)
        output = run_log_printer([container])

        self.assertIn(glyph, output)


def run_log_printer(containers):
    r, w = os.pipe()
    reader, writer = os.fdopen(r, 'r'), os.fdopen(w, 'w')
    printer = LogPrinter(containers, output=writer)
    printer.run()
    writer.close()
    return reader.read()


class MockContainer(object):
    def __init__(self, reader):
        self._reader = reader

    @property
    def name(self):
        return 'myapp_web_1'

    @property
    def name_without_project(self):
        return 'web_1'

    def attach(self, *args, **kwargs):
        return self._reader()

    def wait(self, *args, **kwargs):
        return 0
