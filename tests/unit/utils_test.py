# encoding: utf-8
from __future__ import unicode_literals

from .. import unittest
from compose import utils


class JsonSplitterTestCase(unittest.TestCase):

    def test_json_splitter_no_object(self):
        data = '{"foo": "bar'
        self.assertEqual(utils.json_splitter(data), (None, None))

    def test_json_splitter_with_object(self):
        data = '{"foo": "bar"}\n  \n{"next": "obj"}'
        self.assertEqual(
            utils.json_splitter(data),
            ({'foo': 'bar'}, '{"next": "obj"}')
        )


class StreamAsTextTestCase(unittest.TestCase):

    def test_stream_with_non_utf_unicode_character(self):
        stream = [b'\xed\xf3\xf3']
        output, = utils.stream_as_text(stream)
        assert output == '���'

    def test_stream_with_utf_character(self):
        stream = ['ěĝ'.encode('utf-8')]
        output, = utils.stream_as_text(stream)
        assert output == 'ěĝ'
