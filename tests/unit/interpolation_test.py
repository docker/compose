import unittest

from compose.config.interpolation import BlankDefaultDict as bddict
from compose.config.interpolation import interpolate
from compose.config.interpolation import InvalidInterpolation


class InterpolationTest(unittest.TestCase):
    def test_valid_interpolations(self):
        self.assertEqual(interpolate('$foo', bddict(foo='hi')), 'hi')
        self.assertEqual(interpolate('${foo}', bddict(foo='hi')), 'hi')

        self.assertEqual(interpolate('${subject} love you', bddict(subject='i')), 'i love you')
        self.assertEqual(interpolate('i ${verb} you', bddict(verb='love')), 'i love you')
        self.assertEqual(interpolate('i love ${object}', bddict(object='you')), 'i love you')

    def test_empty_value(self):
        self.assertEqual(interpolate('${foo}', bddict(foo='')), '')

    def test_unset_value(self):
        self.assertEqual(interpolate('${foo}', bddict()), '')

    def test_escaped_interpolation(self):
        self.assertEqual(interpolate('$${foo}', bddict(foo='hi')), '${foo}')

    def test_invalid_strings(self):
        self.assertRaises(InvalidInterpolation, lambda: interpolate('${', bddict()))
        self.assertRaises(InvalidInterpolation, lambda: interpolate('$}', bddict()))
        self.assertRaises(InvalidInterpolation, lambda: interpolate('${}', bddict()))
        self.assertRaises(InvalidInterpolation, lambda: interpolate('${ }', bddict()))
        self.assertRaises(InvalidInterpolation, lambda: interpolate('${ foo}', bddict()))
        self.assertRaises(InvalidInterpolation, lambda: interpolate('${foo }', bddict()))
        self.assertRaises(InvalidInterpolation, lambda: interpolate('${foo!}', bddict()))
