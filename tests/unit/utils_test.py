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
