from __future__ import absolute_import
from __future__ import unicode_literals

import logging

from compose.cli import colors
from compose.cli.formatter import ConsoleWarningFormatter
from tests import unittest


MESSAGE = 'this is the message'


def makeLogRecord(level):
    return logging.LogRecord('name', level, 'pathame', 0, MESSAGE, (), None)


class ConsoleWarningFormatterTestCase(unittest.TestCase):

    def setUp(self):
        self.formatter = ConsoleWarningFormatter()

    def test_format_warn(self):
        output = self.formatter.format(makeLogRecord(logging.WARN))
        expected = colors.yellow('WARNING') + ': '
        assert output == expected + MESSAGE

    def test_format_error(self):
        output = self.formatter.format(makeLogRecord(logging.ERROR))
        expected = colors.red('ERROR') + ': '
        assert output == expected + MESSAGE

    def test_format_info(self):
        output = self.formatter.format(makeLogRecord(logging.INFO))
        assert output == MESSAGE
