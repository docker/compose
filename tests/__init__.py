import sys

if sys.version_info >= (2, 7):
    import unittest  # NOQA
else:
    import unittest2 as unittest  # NOQA

try:
    from unittest import mock
except ImportError:
    import mock  # NOQA
