#!/usr/bin/env python
from __future__ import print_function

import datetime
import os.path
import sys

os.environ['DATE'] = str(datetime.date.today())

for line in sys.stdin:
    print(os.path.expandvars(line), end='')
