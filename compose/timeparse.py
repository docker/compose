#!/usr/bin/env python
# -*- coding: utf-8 -*-
'''
timeparse.py
(c) Will Roberts <wildwilhelm@gmail.com>  1 February, 2014

This is a vendored and modified copy of:
github.com/wroberts/pytimeparse @ cc0550d

It has been modified to mimic the behaviour of
https://golang.org/pkg/time/#ParseDuration
'''
# MIT LICENSE
#
# Permission is hereby granted, free of charge, to any person
# obtaining a copy of this software and associated documentation files
# (the "Software"), to deal in the Software without restriction,
# including without limitation the rights to use, copy, modify, merge,
# publish, distribute, sublicense, and/or sell copies of the Software,
# and to permit persons to whom the Software is furnished to do so,
# subject to the following conditions:
#
# The above copyright notice and this permission notice shall be
# included in all copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
# EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
# MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
# NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS
# BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN
# ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
# CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
# SOFTWARE.
from __future__ import absolute_import
from __future__ import unicode_literals

import re

HOURS = r'(?P<hours>[\d.]+)h'
MINS = r'(?P<mins>[\d.]+)m'
SECS = r'(?P<secs>[\d.]+)s'
MILLI = r'(?P<milli>[\d.]+)ms'
MICRO = r'(?P<micro>[\d.]+)(?:us|µs)'
NANO = r'(?P<nano>[\d.]+)ns'


def opt(x):
    return r'(?:{x})?'.format(x=x)


TIMEFORMAT = r'{HOURS}{MINS}{SECS}{MILLI}{MICRO}{NANO}'.format(
    HOURS=opt(HOURS),
    MINS=opt(MINS),
    SECS=opt(SECS),
    MILLI=opt(MILLI),
    MICRO=opt(MICRO),
    NANO=opt(NANO),
)

MULTIPLIERS = dict([
    ('hours',   60 * 60),
    ('mins',    60),
    ('secs',    1),
    ('milli',   1.0 / 1000),
    ('micro',   1.0 / 1000.0 / 1000),
    ('nano',    1.0 / 1000.0 / 1000.0 / 1000.0),
])


def timeparse(sval):
    """Parse a time expression, returning it as a number of seconds.  If
    possible, the return value will be an `int`; if this is not
    possible, the return will be a `float`.  Returns `None` if a time
    expression cannot be parsed from the given string.

    Arguments:
    - `sval`: the string value to parse

    >>> timeparse('1m24s')
    84
    >>> timeparse('1.2 minutes')
    72
    >>> timeparse('1.2 seconds')
    1.2
    """
    match = re.match(r'\s*' + TIMEFORMAT + r'\s*$', sval, re.I)
    if not match or not match.group(0).strip():
        return

    mdict = match.groupdict()
    return sum(
        MULTIPLIERS[k] * cast(v) for (k, v) in mdict.items() if v is not None)


def cast(value):
    return int(value, 10) if value.isdigit() else float(value)
