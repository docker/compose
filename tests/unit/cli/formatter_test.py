import logging

from compose.cli import colors
from compose.cli.formatter import ConsoleWarningFormatter
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
        assert output == message.decode('utf-8')

    def test_format_unicode_warn(self):
        message = b'\xec\xa0\x95\xec\x88\x98\xec\xa0\x95'
        output = self.formatter.format(make_log_record(logging.WARN, message))
        expected = colors.yellow('WARNING') + ': '
        assert output == '{}{}'.format(expected, message.decode('utf-8'))

    def test_format_unicode_error(self):
        message = b'\xec\xa0\x95\xec\x88\x98\xec\xa0\x95'
        output = self.formatter.format(make_log_record(logging.ERROR, message))
        expected = colors.red('ERROR') + ': '
        assert output == '{}{}'.format(expected, message.decode('utf-8'))
