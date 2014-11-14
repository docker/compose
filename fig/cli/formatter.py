from __future__ import unicode_literals
from __future__ import absolute_import
import os
import texttable


def get_tty_width():
    tty_size = os.popen('stty size', 'r').read().split()
    if len(tty_size) != 2:
        return 80
    _, width = tty_size
    return int(width)


class Formatter(object):
    def table(self, headers, rows):
        table = texttable.Texttable(max_width=get_tty_width())
        table.set_cols_dtype(['t' for h in headers])
        table.add_rows([headers] + rows)
        table.set_deco(table.HEADER)
        table.set_chars(['-', '|', '+', '-'])

        return table.draw()
