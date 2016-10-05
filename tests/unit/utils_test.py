# encoding: utf-8
from __future__ import absolute_import
from __future__ import unicode_literals

from compose import utils


class TestJsonSplitter(object):

    def test_json_splitter_no_object(self):
        data = '{"foo": "bar'
        assert utils.json_splitter(data) is None

    def test_json_splitter_with_object(self):
        data = '{"foo": "bar"}\n  \n{"next": "obj"}'
        assert utils.json_splitter(data) == ({'foo': 'bar'}, '{"next": "obj"}')

    def test_json_splitter_leading_whitespace(self):
        data = '\n   \r{"foo": "bar"}\n\n   {"next": "obj"}'
        assert utils.json_splitter(data) == ({'foo': 'bar'}, '{"next": "obj"}')


class TestStreamAsText(object):

    def test_stream_with_non_utf_unicode_character(self):
        stream = [b'\xed\xf3\xf3']
        output, = utils.stream_as_text(stream)
        assert output == '���'

    def test_stream_with_utf_character(self):
        stream = ['ěĝ'.encode('utf-8')]
        output, = utils.stream_as_text(stream)
        assert output == 'ěĝ'


class TestJsonStream(object):

    def test_with_falsy_entries(self):
        stream = [
            '{"one": "two"}\n{}\n',
            "[1, 2, 3]\n[]\n",
        ]
        output = list(utils.json_stream(stream))
        assert output == [
            {'one': 'two'},
            {},
            [1, 2, 3],
            [],
        ]

    def test_with_leading_whitespace(self):
        stream = [
            '\n  \r\n  {"one": "two"}{"x": 1}',
            '  {"three": "four"}\t\t{"x": 2}'
        ]
        output = list(utils.json_stream(stream))
        assert output == [
            {'one': 'two'},
            {'x': 1},
            {'three': 'four'},
            {'x': 2}
        ]
