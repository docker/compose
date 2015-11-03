from __future__ import absolute_import
from __future__ import unicode_literals

import mock
import six

from compose.cli.log_printer import LogPrinter
from compose.cli.log_printer import wait_on_exit
from compose.container import Container
from tests import unittest


def build_mock_container(reader):
    return mock.Mock(
        spec=Container,
        name='myapp_web_1',
        name_without_project='web_1',
        has_api_logs=True,
        log_stream=None,
        attach=reader,
        wait=mock.Mock(return_value=0),
    )


class LogPrinterTest(unittest.TestCase):
    def get_default_output(self, monochrome=False):
        def reader(*args, **kwargs):
            yield b"hello\nworld"
        container = build_mock_container(reader)
        output = run_log_printer([container], monochrome=monochrome)
        return output

    def test_single_container(self):
        output = self.get_default_output()

        self.assertIn('hello', output)
        self.assertIn('world', output)

    def test_monochrome(self):
        output = self.get_default_output(monochrome=True)
        self.assertNotIn('\033[', output)

    def test_polychrome(self):
        output = self.get_default_output()
        self.assertIn('\033[', output)

    def test_unicode(self):
        glyph = u'\u2022'

        def reader(*args, **kwargs):
            yield glyph.encode('utf-8') + b'\n'

        container = build_mock_container(reader)
        output = run_log_printer([container])
        if six.PY2:
            output = output.decode('utf-8')

        self.assertIn(glyph, output)

    def test_wait_on_exit(self):
        exit_status = 3
        mock_container = mock.Mock(
            spec=Container,
            name='cname',
            wait=mock.Mock(return_value=exit_status))

        expected = '{} exited with code {}\n'.format(mock_container.name, exit_status)
        self.assertEqual(expected, wait_on_exit(mock_container))

    def test_generator_with_no_logs(self):
        mock_container = mock.Mock(
            spec=Container,
            has_api_logs=False,
            log_driver='none',
            name_without_project='web_1',
            wait=mock.Mock(return_value=0))

        output = run_log_printer([mock_container])
        self.assertIn(
            "WARNING: no logs are available with the 'none' log driver\n",
            output
        )


def run_log_printer(containers, monochrome=False):
    output = six.StringIO()
    LogPrinter(containers, output=output, monochrome=monochrome).run()
    return output.getvalue()
