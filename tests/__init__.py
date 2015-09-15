import sys

import mock  # noqa

if sys.version_info >= (2, 7):
    import unittest  # NOQA
else:
    import unittest2 as unittest  # NOQA
