from __future__ import absolute_import
from __future__ import unicode_literals

import pytest
from docker.errors import APIError
from requests.exceptions import ConnectionError

from compose.cli import errors
from compose.cli.errors import handle_connection_errors
from tests import mock


@pytest.yield_fixture
def mock_logging():
    with mock.patch('compose.cli.errors.log', autospec=True) as mock_log:
        yield mock_log


def patch_find_executable(side_effect):
    return mock.patch(
        'compose.cli.errors.find_executable',
        autospec=True,
        side_effect=side_effect)


class TestHandleConnectionErrors(object):

    def test_generic_connection_error(self, mock_logging):
        with pytest.raises(errors.ConnectionError):
            with patch_find_executable(['/bin/docker', None]):
                with handle_connection_errors(mock.Mock()):
                    raise ConnectionError()

        _, args, _ = mock_logging.error.mock_calls[0]
        assert "Couldn't connect to Docker daemon" in args[0]

    def test_api_error_version_mismatch(self, mock_logging):
        with pytest.raises(errors.ConnectionError):
            with handle_connection_errors(mock.Mock(api_version='1.22')):
                raise APIError(None, None, b"client is newer than server")

        _, args, _ = mock_logging.error.mock_calls[0]
        assert "Docker Engine of version 1.10.0 or greater" in args[0]

    def test_api_error_version_mismatch_unicode_explanation(self, mock_logging):
        with pytest.raises(errors.ConnectionError):
            with handle_connection_errors(mock.Mock(api_version='1.22')):
                raise APIError(None, None, u"client is newer than server")

        _, args, _ = mock_logging.error.mock_calls[0]
        assert "Docker Engine of version 1.10.0 or greater" in args[0]

    def test_api_error_version_other(self, mock_logging):
        msg = b"Something broke!"
        with pytest.raises(errors.ConnectionError):
            with handle_connection_errors(mock.Mock(api_version='1.22')):
                raise APIError(None, None, msg)

        mock_logging.error.assert_called_once_with(msg.decode('utf-8'))

    def test_api_error_version_other_unicode_explanation(self, mock_logging):
        msg = u"Something broke!"
        with pytest.raises(errors.ConnectionError):
            with handle_connection_errors(mock.Mock(api_version='1.22')):
                raise APIError(None, None, msg)

        mock_logging.error.assert_called_once_with(msg)
