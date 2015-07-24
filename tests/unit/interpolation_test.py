import unittest

from compose.config.interpolation import interpolate, InvalidInterpolation


class InterpolationTest(unittest.TestCase):
    def test_valid_interpolations(self):
        self.assertEqual(interpolate('$foo', dict(foo='hi')), 'hi')
        self.assertEqual(interpolate('${foo}', dict(foo='hi')), 'hi')

        self.assertEqual(interpolate('${subject} love you', dict(subject='i')), 'i love you')
        self.assertEqual(interpolate('i ${verb} you', dict(verb='love')), 'i love you')
        self.assertEqual(interpolate('i love ${object}', dict(object='you')), 'i love you')

    def test_empty_value(self):
        self.assertEqual(interpolate('${foo}', dict(foo='')), '')

    def test_unset_value(self):
        self.assertEqual(interpolate('${foo}', dict()), '')

    def test_escaped_interpolation(self):
        self.assertEqual(interpolate('$${foo}', dict(foo='hi')), '${foo}')

    def test_invalid_strings(self):
        self.assertRaises(InvalidInterpolation, lambda: interpolate('${', dict()))
        self.assertRaises(InvalidInterpolation, lambda: interpolate('$}', dict()))
        self.assertRaises(InvalidInterpolation, lambda: interpolate('${}', dict()))
        self.assertRaises(InvalidInterpolation, lambda: interpolate('${ }', dict()))
        self.assertRaises(InvalidInterpolation, lambda: interpolate('${ foo}', dict()))
        self.assertRaises(InvalidInterpolation, lambda: interpolate('${foo }', dict()))
        self.assertRaises(InvalidInterpolation, lambda: interpolate('${foo!}', dict()))
