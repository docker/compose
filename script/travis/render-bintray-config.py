#!/usr/bin/env python
from __future__ import absolute_import
from __future__ import print_function
from __future__ import unicode_literals

import datetime
import os.path
import sys

os.environ['DATE'] = str(datetime.date.today())

for line in sys.stdin:
    print(os.path.expandvars(line), end='')
