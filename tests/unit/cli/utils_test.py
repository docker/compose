from __future__ import absolute_import
from __future__ import unicode_literals

import unittest
from datetime import datetime
from datetime import timedelta

from compose.cli.utils import get_datetime_from_timestamp_or_duration
from compose.cli.utils import human_readable_file_size
from compose.utils import unquote_path


class UnquotePathTest(unittest.TestCase):
    def test_no_quotes(self):
        assert unquote_path('hello') == 'hello'

    def test_simple_quotes(self):
        assert unquote_path('"hello"') == 'hello'

    def test_uneven_quotes(self):
        assert unquote_path('"hello') == '"hello'
        assert unquote_path('hello"') == 'hello"'

    def test_nested_quotes(self):
        assert unquote_path('""hello""') == '"hello"'
        assert unquote_path('"hel"lo"') == 'hel"lo'
        assert unquote_path('"hello""') == 'hello"'


class HumanReadableFileSizeTest(unittest.TestCase):
    def test_100b(self):
        assert human_readable_file_size(100) == '100 B'

    def test_1kb(self):
        assert human_readable_file_size(1000) == '1 kB'
        assert human_readable_file_size(1024) == '1.024 kB'

    def test_1023b(self):
        assert human_readable_file_size(1023) == '1.023 kB'

    def test_999b(self):
        assert human_readable_file_size(999) == '999 B'

    def test_units(self):
        assert human_readable_file_size((10 ** 3) ** 0) == '1 B'
        assert human_readable_file_size((10 ** 3) ** 1) == '1 kB'
        assert human_readable_file_size((10 ** 3) ** 2) == '1 MB'
        assert human_readable_file_size((10 ** 3) ** 3) == '1 GB'
        assert human_readable_file_size((10 ** 3) ** 4) == '1 TB'
        assert human_readable_file_size((10 ** 3) ** 5) == '1 PB'
        assert human_readable_file_size((10 ** 3) ** 6) == '1 EB'


class GetDatetimeFromTimestampOrDuration(unittest.TestCase):
    def test_duration(self):
        expected = datetime.now() - timedelta(hours=48, minutes=30, seconds=30)
        value = get_datetime_from_timestamp_or_duration('48h30m30s')
        assert expected - value < timedelta(seconds=2)

    def test_timestamp(self):
        assert get_datetime_from_timestamp_or_duration('2020-01-13T17:00:00') == \
            datetime(year=2020, month=1, day=13, hour=17, minute=0, second=0)
