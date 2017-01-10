from __future__ import absolute_import
from __future__ import unicode_literals

import logging
import os

import six
import texttable

from compose.cli import colors


def get_tty_width():
    tty_size = os.popen('stty size', 'r').read().split()
    if len(tty_size) != 2:
        return 0
    _, width = tty_size
    return int(width)


class Formatter(object):
    """Format tabular data for printing."""
    def table(self, headers, rows):
        table = texttable.Texttable(max_width=get_tty_width())
        table.set_cols_dtype(['t' for h in headers])
        table.add_rows([headers] + rows)
        table.set_deco(table.HEADER)
        table.set_chars(['-', '|', '+', '-'])

        return table.draw()

    def clear(self):
        return u"\u001b[2J" + u"\u001b[0;0H"

    def percentage(self, n):
        return str(round(n, 2)) + "%"

    def sizeof(self, num, suffix='B', binary=True):
        units = ['', 'Ki', 'Mi', 'Gi', 'Ti', 'Pi', 'Ei', 'Zi'] if binary \
            else ['', 'K', 'M', 'G', 'T', 'P', 'E', 'Z']
        base = 1024.0 if binary else 1000.0
        for unit in units:
            if abs(num) < base:
                return "%3.2f %s%s" % (num, unit, suffix)
            num /= base
        lastUnit = 'Yi' if binary else 'Y'
        return "%.2f %s%s" % (num, lastUnit, suffix)


class ConsoleWarningFormatter(logging.Formatter):
    """A logging.Formatter which prints WARNING and ERROR messages with
    a prefix of the log level colored appropriate for the log level.
    """

    def get_level_message(self, record):
        separator = ': '
        if record.levelno == logging.WARNING:
            return colors.yellow(record.levelname) + separator
        if record.levelno == logging.ERROR:
            return colors.red(record.levelname) + separator

        return ''

    def format(self, record):
        if isinstance(record.msg, six.binary_type):
            record.msg = record.msg.decode('utf-8')
        message = super(ConsoleWarningFormatter, self).format(record)
        return '{0}{1}'.format(self.get_level_message(record), message)
