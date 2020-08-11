import pytest
from docker.errors import APIError
from requests.exceptions import ConnectionError

from compose.cli import errors
from compose.cli.errors import handle_connection_errors
from compose.const import IS_WINDOWS_PLATFORM
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


class TestHandleConnectionErrors:

    def test_generic_connection_error(self, mock_logging):
        with pytest.raises(errors.ConnectionError):
            with patch_find_executable(['/bin/docker', None]):
                with handle_connection_errors(mock.Mock()):
                    raise ConnectionError()

        _, args, _ = mock_logging.error.mock_calls[0]
        assert "Couldn't connect to Docker daemon" in args[0]

    def test_api_error_version_mismatch(self, mock_logging):
        with pytest.raises(errors.ConnectionError):
            with handle_connection_errors(mock.Mock(api_version='1.38')):
                raise APIError(None, None, b"client is newer than server")

        _, args, _ = mock_logging.error.mock_calls[0]
        assert "Docker Engine of version 18.06.0 or greater" in args[0]

    def test_api_error_version_mismatch_unicode_explanation(self, mock_logging):
        with pytest.raises(errors.ConnectionError):
            with handle_connection_errors(mock.Mock(api_version='1.38')):
                raise APIError(None, None, "client is newer than server")

        _, args, _ = mock_logging.error.mock_calls[0]
        assert "Docker Engine of version 18.06.0 or greater" in args[0]

    def test_api_error_version_other(self, mock_logging):
        msg = b"Something broke!"
        with pytest.raises(errors.ConnectionError):
            with handle_connection_errors(mock.Mock(api_version='1.22')):
                raise APIError(None, None, msg)

        mock_logging.error.assert_called_once_with(msg.decode('utf-8'))

    def test_api_error_version_other_unicode_explanation(self, mock_logging):
        msg = "Something broke!"
        with pytest.raises(errors.ConnectionError):
            with handle_connection_errors(mock.Mock(api_version='1.22')):
                raise APIError(None, None, msg)

        mock_logging.error.assert_called_once_with(msg)

    @pytest.mark.skipif(not IS_WINDOWS_PLATFORM, reason='Needs pywin32')
    def test_windows_pipe_error_no_data(self, mock_logging):
        import pywintypes
        with pytest.raises(errors.ConnectionError):
            with handle_connection_errors(mock.Mock(api_version='1.22')):
                raise pywintypes.error(232, 'WriteFile', 'The pipe is being closed.')

        _, args, _ = mock_logging.error.mock_calls[0]
        assert "The current Compose file version is not compatible with your engine version." in args[0]

    @pytest.mark.skipif(not IS_WINDOWS_PLATFORM, reason='Needs pywin32')
    def test_windows_pipe_error_misc(self, mock_logging):
        import pywintypes
        with pytest.raises(errors.ConnectionError):
            with handle_connection_errors(mock.Mock(api_version='1.22')):
                raise pywintypes.error(231, 'WriteFile', 'The pipe is busy.')

        _, args, _ = mock_logging.error.mock_calls[0]
        assert "Windows named pipe error: The pipe is busy. (code: 231)" == args[0]

    @pytest.mark.skipif(not IS_WINDOWS_PLATFORM, reason='Needs pywin32')
    def test_windows_pipe_error_encoding_issue(self, mock_logging):
        import pywintypes
        with pytest.raises(errors.ConnectionError):
            with handle_connection_errors(mock.Mock(api_version='1.22')):
                raise pywintypes.error(9999, 'WriteFile', 'I use weird characters \xe9')

        _, args, _ = mock_logging.error.mock_calls[0]
        assert 'Windows named pipe error: I use weird characters \xe9 (code: 9999)' == args[0]
