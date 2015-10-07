from __future__ import absolute_import

import pytest
from requests.exceptions import ConnectionError

from compose.cli import errors
from compose.cli.command import friendly_error_message
from tests import mock
from tests import unittest


class FriendlyErrorMessageTestCase(unittest.TestCase):

    def test_dispatch_generic_connection_error(self):
        with pytest.raises(errors.ConnectionErrorGeneric):
            with mock.patch(
                'compose.cli.command.call_silently',
                autospec=True,
                side_effect=[0, 1]
            ):
                with friendly_error_message():
                    raise ConnectionError()
