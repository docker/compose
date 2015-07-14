from __future__ import unicode_literals
from __future__ import absolute_import
import os
import texttable

class Formatter(object):
    def table(self, headers, rows):
        table = texttable.Texttable(max_width=0)
        table.set_cols_dtype(['t' for h in headers])
        table.add_rows([headers] + rows)
        table.set_deco(table.HEADER)
        table.set_chars(['-', '|', '+', '-'])

        return table.draw()
