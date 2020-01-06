from __future__ import absolute_import
from __future__ import unicode_literals

import logging
import shutil

import six
import texttable

from compose.cli import colors

if hasattr(shutil, "get_terminal_size"):
    from shutil import get_terminal_size
else:
    from backports.shutil_get_terminal_size import get_terminal_size


def get_tty_width():
    try:
        # get_terminal_size can't determine the size if compose is piped
        # to another command. But in such case it doesn't make sense to
        # try format the output by terminal size as this output is consumed
        # by another command. So let's pretend we have a huge terminal so
        # output is single-lined
        width, _ = get_terminal_size(fallback=(999, 0))
        return int(width)
    except OSError:
        return 0


class Formatter:
    """Format tabular data for printing."""

    @staticmethod
    def table(headers, rows):
        table = texttable.Texttable(max_width=get_tty_width())
        table.set_cols_dtype(['t' for h in headers])
        table.add_rows([headers] + rows)
        table.set_deco(table.HEADER)
        table.set_chars(['-', '|', '+', '-'])

        return table.draw()


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
