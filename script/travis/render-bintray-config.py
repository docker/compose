#!/usr/bin/env python
import datetime
import os.path
import sys

os.environ['DATE'] = str(datetime.date.today())

for line in sys.stdin:
    print os.path.expandvars(line),
