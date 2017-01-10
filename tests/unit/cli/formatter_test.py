from __future__ import absolute_import
from __future__ import unicode_literals

import logging

from compose.cli import colors
from compose.cli.formatter import ConsoleWarningFormatter
from compose.cli.formatter import Formatter
from tests import unittest


MESSAGE = 'this is the message'


def make_log_record(level, message=None):
    return logging.LogRecord('name', level, 'pathame', 0, message or MESSAGE, (), None)


class ConsoleWarningFormatterTestCase(unittest.TestCase):

    def setUp(self):
        self.formatter = ConsoleWarningFormatter()

    def test_format_warn(self):
        output = self.formatter.format(make_log_record(logging.WARN))
        expected = colors.yellow('WARNING') + ': '
        assert output == expected + MESSAGE

    def test_format_error(self):
        output = self.formatter.format(make_log_record(logging.ERROR))
        expected = colors.red('ERROR') + ': '
        assert output == expected + MESSAGE

    def test_format_info(self):
        output = self.formatter.format(make_log_record(logging.INFO))
        assert output == MESSAGE

    def test_format_unicode_info(self):
        message = b'\xec\xa0\x95\xec\x88\x98\xec\xa0\x95'
        output = self.formatter.format(make_log_record(logging.INFO, message))
        print(output)
        assert output == message.decode('utf-8')

    def test_format_unicode_warn(self):
        message = b'\xec\xa0\x95\xec\x88\x98\xec\xa0\x95'
        output = self.formatter.format(make_log_record(logging.WARN, message))
        expected = colors.yellow('WARNING') + ': '
        assert output == '{0}{1}'.format(expected, message.decode('utf-8'))

    def test_format_unicode_error(self):
        message = b'\xec\xa0\x95\xec\x88\x98\xec\xa0\x95'
        output = self.formatter.format(make_log_record(logging.ERROR, message))
        expected = colors.red('ERROR') + ': '
        assert output == '{0}{1}'.format(expected, message.decode('utf-8'))


class FormatterTestCase(unittest.TestCase):

    def setUp(self):
        self.formatter = Formatter()

    def test_format_percentage(self):
        self.assertEqual("12.34%", self.formatter.percentage(12.3445))

    def test_format_sizeof(self):
        self.assertEqual("120.00 B", self.formatter.sizeof(120))
        self.assertEqual("120.00 MiB", self.formatter.sizeof(120*1024*1024))
        self.assertEqual("120.00 GiB", self.formatter.sizeof(120*1024*1024*1024))
        self.assertEqual("120.00 TiB", self.formatter.sizeof(120 * 1024 * 1024 * 1024 * 1024))

    def test_format_sizeof_not_binary(self):
        self.assertEqual("120.00 B", self.formatter.sizeof(120, binary=False))
        self.assertEqual("120.00 MB", self.formatter.sizeof(120*1000*1000, binary=False))
        self.assertEqual("120.00 GB", self.formatter.sizeof(120*1000*1000*1000, binary=False))
        self.assertEqual("120.00 TB",
                         self.formatter.sizeof(120 * 1000 * 1000 * 1000 * 1000, binary=False))

    def test_clear(self):
        self.assertEqual(u"\u001b[2J\u001b[0;0H", self.formatter.clear())
